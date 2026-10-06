// Package handler_test contains integration-style security and behaviour tests
// for the HTTP handlers. Tests use an in-memory DB and a test JWT service so
// they require CGO (sqlite3); they skip automatically when CGO is disabled.
package handler_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"kestrel/internal/auth"
	"kestrel/internal/config"
	"kestrel/internal/database"
	"kestrel/internal/handler"
	"kestrel/internal/middleware"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// ── test helpers ──────────────────────────────────────────────────────────────

func setupDB(t *testing.T) *database.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := database.New(filepath.Join(dir, "test.db"), zap.NewNop())
	if err != nil {
		if strings.Contains(err.Error(), "CGO_ENABLED=0") {
			t.Skip("handler tests require CGO (sqlite3)")
		}
		t.Fatalf("database.New() error: %v", err)
	}
	t.Cleanup(func() {
		db.Close()
		os.RemoveAll(dir)
	})
	return db
}

func setupAuthRouter(t *testing.T) (*gin.Engine, *database.DB, *auth.Service) {
	t.Helper()
	db := setupDB(t)
	cfg := &config.Config{
		Auth: config.AuthConfig{
			JWTSecret:            "test-secret-32-bytes-padded-1234",
			SessionDurationHours: 1,
		},
		RateLimit: config.RateLimitConfig{
			LoginMaxAttempts: 10,
			LoginWindowSecs:  60,
		},
	}
	authSvc := auth.New(db, cfg.Auth.JWTSecret, cfg.Auth.SessionDurationHours)
	loginRL := middleware.NewRateLimiter(cfg.RateLimit.LoginMaxAttempts, cfg.RateLimit.LoginWindowSecs)
	authH := handler.NewAuthHandler(authSvc, db)

	r := gin.New()
	r.Use(middleware.SecurityHeaders())
	r.POST("/api/auth/login", middleware.LoginRateLimit(loginRL), authH.Login)
	r.POST("/api/auth/logout", middleware.Auth(authSvc), authH.Logout)
	r.GET("/api/auth/me", middleware.Auth(authSvc), authH.Me)
	r.POST("/api/auth/change-password", middleware.Auth(authSvc), authH.ChangePassword)
	return r, db, authSvc
}

func makeToken(t *testing.T, svc *auth.Service, userID string) string {
	t.Helper()
	token, err := svc.IssueToken(userID, "sess-1")
	if err != nil {
		t.Fatalf("IssueToken() error: %v", err)
	}
	return token
}

func do(t *testing.T, router http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	t.Helper()
	var bodyReader *bytes.Reader
	if body != "" {
		bodyReader = bytes.NewReader([]byte(body))
	} else {
		bodyReader = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, path, bodyReader)
	if err != nil {
		t.Fatalf("NewRequest() error: %v", err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// ── Auth: login / logout / me ────────────────────────────────────────────────

func TestLoginSuccess(t *testing.T) {
	router, db, _ := setupAuthRouter(t)
	hash, _ := auth.HashPassword("P@ssw0rd!")
	db.CreateUser("testuser", hash, "", "Test User")

	w := do(t, router, "POST", "/api/auth/login",
		`{"username":"testuser","password":"P@ssw0rd!"}`, "")
	if w.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if _, ok := resp["token"]; !ok {
		t.Error("login response missing 'token' field")
	}
}

func TestLoginWrongPassword(t *testing.T) {
	router, db, _ := setupAuthRouter(t)
	hash, _ := auth.HashPassword("correct")
	db.CreateUser("victim", hash, "", "")

	w := do(t, router, "POST", "/api/auth/login",
		`{"username":"victim","password":"wrong"}`, "")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("wrong password status = %d, want 401", w.Code)
	}
}

func TestLoginNonexistentUser(t *testing.T) {
	router, _, _ := setupAuthRouter(t)
	w := do(t, router, "POST", "/api/auth/login",
		`{"username":"nobody","password":"pass"}`, "")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("nonexistent user status = %d, want 401", w.Code)
	}
}

func TestLoginMissingFields(t *testing.T) {
	router, _, _ := setupAuthRouter(t)
	w := do(t, router, "POST", "/api/auth/login", `{}`, "")
	if w.Code == http.StatusOK {
		t.Error("empty credentials should not succeed")
	}
}

// ── Auth: JWT Bearer authentication ──────────────────────────────────────────

func TestMeRequiresToken(t *testing.T) {
	router, _, _ := setupAuthRouter(t)
	w := do(t, router, "GET", "/api/auth/me", "", "")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated /me status = %d, want 401", w.Code)
	}
}

func TestMeWithValidToken(t *testing.T) {
	router, db, svc := setupAuthRouter(t)
	hash, _ := auth.HashPassword("pw")
	user, _ := db.CreateUser("meuser", hash, "", "Me User")

	// Create a DB session for the token.
	db.Exec(`INSERT INTO sessions (id,user_id,token_hash,created_at,expires_at,last_seen_at)
		VALUES ('s1',?,'thash',?,?,?)`,
		user.ID, time.Now(), time.Now().Add(time.Hour), time.Now())

	token := makeToken(t, svc, user.ID)
	w := do(t, router, "GET", "/api/auth/me", "", token)
	if w.Code != http.StatusOK {
		t.Errorf("/me with valid token = %d, want 200", w.Code)
	}
}

func TestMeWithInvalidToken(t *testing.T) {
	router, _, _ := setupAuthRouter(t)
	w := do(t, router, "GET", "/api/auth/me", "", "totally.invalid.token")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("invalid token /me = %d, want 401", w.Code)
	}
}

func TestMeWithTamperedToken(t *testing.T) {
	router, db, svc := setupAuthRouter(t)
	hash, _ := auth.HashPassword("pw")
	user, _ := db.CreateUser("tampuser", hash, "", "")

	token := makeToken(t, svc, user.ID)
	// Flip last character to corrupt the signature.
	tampered := token[:len(token)-1] + "X"
	w := do(t, router, "GET", "/api/auth/me", "", tampered)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("tampered token /me = %d, want 401", w.Code)
	}
}

// ── Security headers are present on all responses ────────────────────────────

func TestSecurityHeadersOnLoginEndpoint(t *testing.T) {
	router, _, _ := setupAuthRouter(t)
	w := do(t, router, "POST", "/api/auth/login", `{}`, "")

	required := []string{"X-Content-Type-Options", "X-Frame-Options", "X-XSS-Protection"}
	for _, h := range required {
		if w.Header().Get(h) == "" {
			t.Errorf("missing security header: %s", h)
		}
	}
}

// ── Rate limit on login ───────────────────────────────────────────────────────

func TestLoginRateLimitEnforced(t *testing.T) {
	_, db, _ := setupAuthRouter(t)
	hash, _ := auth.HashPassword("pw")
	db.CreateUser("rluser", hash, "", "")

	// Build a fresh router with a very tight limit.
	authSvc := auth.New(db, "test-secret-32-bytes-padded-xxxx", 1)
	loginRL := middleware.NewRateLimiter(3, 60)
	authH := handler.NewAuthHandler(authSvc, db)
	r := gin.New()
	r.POST("/api/auth/login", middleware.LoginRateLimit(loginRL), authH.Login)

	attempt := func() int {
		w := do(t, r, "POST", "/api/auth/login",
			`{"username":"rluser","password":"wrong"}`, "")
		return w.Code
	}

	attempt(); attempt(); attempt()
	if code := attempt(); code != http.StatusTooManyRequests {
		t.Errorf("4th attempt status = %d, want 429", code)
	}
}

// ── Password change ───────────────────────────────────────────────────────────

func TestChangePasswordSuccess(t *testing.T) {
	router, db, svc := setupAuthRouter(t)
	hash, _ := auth.HashPassword("OldP@ssw0rd!99")
	user, _ := db.CreateUser("changeuser", hash, "", "")

	db.Exec(`INSERT INTO sessions (id,user_id,token_hash,created_at,expires_at,last_seen_at)
		VALUES ('s2',?,'thash2',?,?,?)`,
		user.ID, time.Now(), time.Now().Add(time.Hour), time.Now())

	token := makeToken(t, svc, user.ID)
	w := do(t, router, "POST", "/api/auth/change-password",
		`{"current_password":"OldP@ssw0rd!99","new_password":"N3wP@ssw0rd!99"}`, token)
	if w.Code != http.StatusOK {
		t.Errorf("change-password status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestChangePasswordWrongCurrent(t *testing.T) {
	router, db, svc := setupAuthRouter(t)
	hash, _ := auth.HashPassword("R3alP@ssw0rd!99")
	user, _ := db.CreateUser("chguser2", hash, "", "")

	db.Exec(`INSERT INTO sessions (id,user_id,token_hash,created_at,expires_at,last_seen_at)
		VALUES ('s3',?,'thash3',?,?,?)`,
		user.ID, time.Now(), time.Now().Add(time.Hour), time.Now())

	token := makeToken(t, svc, user.ID)
	w := do(t, router, "POST", "/api/auth/change-password",
		`{"current_password":"Wr0ngP@ss!99","new_password":"N3wP@ssw0rd!99"}`, token)
	if w.Code != http.StatusUnauthorized && w.Code != http.StatusBadRequest {
		t.Errorf("wrong current password status = %d, want 400 or 401", w.Code)
	}
}

// ── SQL injection probe ───────────────────────────────────────────────────────

func TestLoginSQLInjectionProbe(t *testing.T) {
	router, db, _ := setupAuthRouter(t)
	hash, _ := auth.HashPassword("safe")
	db.CreateUser("safeuser", hash, "", "")

	// Common SQL injection payloads should not authenticate.
	payloads := []string{
		`{"username":"' OR '1'='1","password":"anything"}`,
		`{"username":"admin'--","password":"pass"}`,
		`{"username":"\" OR 1=1--","password":"pass"}`,
		fmt.Sprintf(`{"username":"%s","password":"x"}`, strings.Repeat("A", 10000)),
	}
	for _, p := range payloads {
		w := do(t, router, "POST", "/api/auth/login", p, "")
		if w.Code == http.StatusOK {
			t.Errorf("SQLi payload %q unexpectedly authenticated (status 200)", p[:min(len(p), 60)])
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ── User handler ──────────────────────────────────────────────────────────────

func TestListUsersRequiresAuth(t *testing.T) {
	db := setupDB(t)
	authSvc := auth.New(db, "secret-32-bytes-exactly-padded!!", 1)
	userH := handler.NewUserHandler(db)

	r := gin.New()
	r.GET("/api/users", middleware.Auth(authSvc), userH.ListUsers)

	w := do(t, r, "GET", "/api/users", "", "")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated /users status = %d, want 401", w.Code)
	}
}

func TestListUsersWithValidToken(t *testing.T) {
	db := setupDB(t)
	authSvc := auth.New(db, "secret-32-bytes-exactly-padded!!", 1)
	userH := handler.NewUserHandler(db)

	r := gin.New()
	r.GET("/api/users", middleware.Auth(authSvc), userH.ListUsers)

	hash, _ := auth.HashPassword("pw")
	user, _ := db.CreateUser("listuser", hash, "", "")
	db.Exec(`INSERT INTO sessions (id,user_id,token_hash,created_at,expires_at,last_seen_at)
		VALUES ('ls1',?,'lhash',?,?,?)`,
		user.ID, time.Now(), time.Now().Add(time.Hour), time.Now())

	token := makeToken(t, authSvc, user.ID)
	w := do(t, r, "GET", "/api/users", "", token)
	if w.Code != http.StatusOK {
		t.Errorf("/users with auth status = %d, want 200", w.Code)
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if _, ok := resp["users"]; !ok {
		t.Error("response missing 'users' field")
	}
}

// ── Must-change-password gate ─────────────────────────────────────────────────

func TestMustChangePasswordGate(t *testing.T) {
	db := setupDB(t)
	authSvc := auth.New(db, "test-secret-32-bytes-padded-1234", 1)
	userH := handler.NewUserHandler(db)

	r := gin.New()
	r.GET("/api/users",
		middleware.Auth(authSvc),
		middleware.RequirePasswordChange(),
		userH.ListUsers,
	)

	hash, _ := auth.HashPassword("pw")
	user, _ := db.CreateUser("mustchg", hash, "", "")
	// must_change_password defaults to 1 in the schema.
	db.Exec(`INSERT INTO sessions (id,user_id,token_hash,created_at,expires_at,last_seen_at)
		VALUES ('mc1',?,'mchash',?,?,?)`,
		user.ID, time.Now(), time.Now().Add(time.Hour), time.Now())

	token := makeToken(t, authSvc, user.ID)
	w := do(t, r, "GET", "/api/users", "", token)
	if w.Code != http.StatusForbidden {
		t.Errorf("must-change-password gate = %d, want 403", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "MUST_CHANGE_PASSWORD") {
		t.Errorf("response missing MUST_CHANGE_PASSWORD code, got: %s", body)
	}
}

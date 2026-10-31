package security

import (
	"database/sql"
	"os"
	"errors"
	"strings"
	"sync"
	"time"

	"kestrel/internal/database"

	"github.com/google/uuid"
)

// Predefined errors for authentication operations.
var (
	ErrInvalidPassword = errors.New("invalid password")
)

// Session represents an authenticated user session.
type Session struct {
	Token              string
	ExpiresAt          time.Time
	UserID             string
	Username           string
	DisplayName        string
	MustChangePassword bool
	Roles              []string
	Permissions        map[string]bool
	PermissionScopes   map[string]string
	Scope              string
}

// AuthManager manages password-based authentication and session lifecycle.
type AuthManager struct {
	sessionDuration time.Duration
	jwtSecret       []byte
	db              *database.DB

	mu                     sync.RWMutex
	sessions               map[string]Session
	revokedJTIs            map[string]time.Time
	globalRevocationCutoff time.Time
}

// NewAuthManager creates a new AuthManager instance.
func NewAuthManager(sessionDurationHours int, secret ...string) *AuthManager {
	if sessionDurationHours <= 0 {
		sessionDurationHours = 12
	}

	var jwtKey []byte
	if len(secret) > 0 && strings.TrimSpace(secret[0]) != "" {
		jwtKey = []byte(strings.TrimSpace(secret[0]))
	} else if envSecret := strings.TrimSpace(os.Getenv("JWT_SECRET")); envSecret != "" {
		jwtKey = []byte(envSecret)
	} else {
		key, err := GenerateRandomKey(32)
		if err != nil {
			jwtKey = []byte("kestrel-default-session-secret-key-32b")
		} else {
			jwtKey = key
		}
	}

	return &AuthManager{
		sessionDuration: time.Duration(sessionDurationHours) * time.Hour,
		jwtSecret:       jwtKey,
		sessions:        make(map[string]Session),
		revokedJTIs:     make(map[string]time.Time),
	}
}

func (a *AuthManager) SetJWTSecret(secret string) {
	if s := strings.TrimSpace(secret); s != "" {
		a.mu.Lock()
		a.jwtSecret = []byte(s)
		a.mu.Unlock()
	}
}

func (a *AuthManager) JWTSecret() []byte {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.jwtSecret
}

// AttachRBACStore enables multi-user RBAC authentication. When no users exist yet,
// it bootstraps the built-in admin account and returns the generated initial password.
func (a *AuthManager) AttachRBACStore(db *database.DB) (generatedAdminPassword string, err error) {
	if db == nil {
		return "", errors.New("database is required for authentication")
	}

	needsAdminPassword, err := db.RBACNeedsAdminPassword()
	if err != nil {
		return "", err
	}

	adminPasswordHash := ""
	if needsAdminPassword {
		generatedAdminPassword, err = GenerateStrongPassword(24)
		if err != nil {
			return "", err
		}
		adminPasswordHash, err = HashPassword(generatedAdminPassword)
		if err != nil {
			return "", err
		}
	}

	if err := db.BootstrapRBAC(adminPasswordHash, PermissionCatalog); err != nil {
		return "", err
	}

	a.mu.Lock()
	a.db = db
	a.mu.Unlock()
	return generatedAdminPassword, nil
}

// Authenticate validates the password and creates a new session.
func (a *AuthManager) Authenticate(username, password string) (string, time.Time, error) {
	session, err := a.authenticateSession(username, password)
	if err != nil {
		return "", time.Time{}, err
	}

	jti := uuid.NewString()
	claims := JWTClaims{
		Subject:            session.UserID,
		Username:           session.Username,
		DisplayName:        session.DisplayName,
		MustChangePassword: session.MustChangePassword,
		Roles:              session.Roles,
		Permissions:        session.Permissions,
		PermissionScopes:   session.PermissionScopes,
		Scope:              session.Scope,
		ID:                 jti,
		IssuedAt:           time.Now().Unix(),
		ExpiresAt:          session.ExpiresAt.Unix(),
	}

	signedToken, err := SignJWT(claims, a.jwtSecret)
	if err != nil {
		signedToken = session.Token
	}

	session.Token = signedToken

	a.mu.Lock()
	a.sessions[signedToken] = session
	a.mu.Unlock()
	return session.Token, session.ExpiresAt, nil
}

func (a *AuthManager) authenticateSession(username, password string) (Session, error) {
	token := uuid.NewString()
	expiresAt := time.Now().Add(a.sessionDuration)

	a.mu.RLock()
	db := a.db
	a.mu.RUnlock()
	if db == nil {
		return Session{}, errors.New("authentication store is not configured")
	}

	username = strings.TrimSpace(strings.ToLower(username))
	if username == "" {
		username = "admin"
	}
	user, err := db.GetRBACUserByUsername(username)
	if err != nil {
		if err == sql.ErrNoRows {
			return Session{}, ErrInvalidPassword
		}
		return Session{}, err
	}
	if !user.Enabled || !VerifyPasswordHash(password, user.PasswordHash) {
		return Session{}, ErrInvalidPassword
	}
	access, err := db.ResolveRBACAccess(user.ID)
	if err != nil {
		return Session{}, err
	}
	roleIDs := make([]string, 0, len(access.Roles))
	for _, role := range access.Roles {
		roleIDs = append(roleIDs, role.ID)
	}
	return Session{
		Token:              token,
		ExpiresAt:          expiresAt,
		UserID:             user.ID,
		Username:           user.Username,
		DisplayName:        user.DisplayName,
		MustChangePassword: user.MustChangePassword,
		Roles:              roleIDs,
		Permissions:        access.Permissions,
		PermissionScopes:   access.PermissionScopes,
		Scope:              access.Scope,
	}, nil
}

func (s Session) ScopeFor(permission string) string {
	if scope := strings.TrimSpace(s.PermissionScopes[strings.TrimSpace(permission)]); scope != "" {
		return scope
	}
	return strings.TrimSpace(s.Scope)
}

// ValidateToken checks whether the provided token is still valid, enforcing DB revocation and user state.
func (a *AuthManager) ValidateToken(token string) (Session, bool) {
	token = strings.TrimSpace(token)
	if token == "" {
		return Session{}, false
	}

	a.mu.RLock()
	secret := a.jwtSecret
	cutoff := a.globalRevocationCutoff
	if _, revoked := a.revokedJTIs[token]; revoked {
		a.mu.RUnlock()
		return Session{}, false
	}
	session, inMem := a.sessions[token]
	db := a.db
	a.mu.RUnlock()

	if inMem {
		if time.Now().After(session.ExpiresAt) {
			return Session{}, false
		}
	}

	// Verify cryptographic signature and expiration envelope
	claims, err := VerifyJWT(token, secret)
	if err != nil || claims == nil {
		return Session{}, false
	}

	// Check global revocation cutoff (e.g. from RevokeAllSessions)
	if !cutoff.IsZero() && claims.IssuedAt <= cutoff.Unix() {
		return Session{}, false
	}

	// Check in-memory JTI revocation
	a.mu.RLock()
	if _, revoked := a.revokedJTIs[claims.ID]; revoked {
		a.mu.RUnlock()
		return Session{}, false
	}
	a.mu.RUnlock()

	// If DB is attached, enforce persistent revocation and user account validity
	if db != nil {
		if db.IsTokenRevoked(claims.ID) || db.IsTokenRevoked(token) {
			return Session{}, false
		}
		user, err := db.GetRBACUserByID(claims.Subject)
		if err != nil || user == nil || !user.Enabled {
			return Session{}, false
		}
		// If password was reset or user modified after token was issued, invalidate immediately
		if claims.IssuedAt < user.UpdatedAt.Unix()-1 {
			return Session{}, false
		}
	}

	mustChange := claims.MustChangePassword
	if db != nil {
		if u, err := db.GetRBACUserByID(claims.Subject); err == nil && u != nil {
			mustChange = u.MustChangePassword
		}
	}

	session = Session{
		Token:              token,
		ExpiresAt:          time.Unix(claims.ExpiresAt, 0),
		UserID:             claims.Subject,
		Username:           claims.Username,
		DisplayName:        claims.DisplayName,
		MustChangePassword: mustChange,
		Roles:              claims.Roles,
		Permissions:        claims.Permissions,
		PermissionScopes:   claims.PermissionScopes,
		Scope:              claims.Scope,
	}

	a.mu.Lock()
	a.sessions[token] = session
	a.mu.Unlock()

	return session, true
}

// CheckPassword verifies whether the provided password matches the current password.
func (a *AuthManager) CheckPassword(password string) bool {
	return a.CheckUserPassword("admin", password)
}

// CheckUserPassword verifies whether the provided password matches a user.
func (a *AuthManager) CheckUserPassword(username, password string) bool {
	a.mu.RLock()
	db := a.db
	a.mu.RUnlock()
	if db == nil {
		return false
	}
	user, err := db.GetRBACUserByUsername(username)
	if err != nil {
		return false
	}
	return VerifyPasswordHash(password, user.PasswordHash)
}

func (a *AuthManager) UpdateUserPassword(userID, password string) error {
	password = strings.TrimSpace(password)
	if password == "" {
		return errors.New("auth password must be configured")
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	a.mu.RLock()
	db := a.db
	a.mu.RUnlock()
	if db == nil {
		return errors.New("authentication store is not configured")
	}
	if err := db.UpdateRBACUserPassword(userID, hash); err != nil {
		return err
	}
	a.mu.Lock()
	for token, session := range a.sessions {
		if session.UserID == userID {
			delete(a.sessions, token)
		}
	}
	a.mu.Unlock()
	return nil
}

// RevokeToken invalidates the specified token and stores revocation in DB and memory.
func (a *AuthManager) RevokeToken(token string) {
	token = strings.TrimSpace(token)
	if token == "" {
		return
	}

	a.mu.Lock()
	delete(a.sessions, token)
	if a.revokedJTIs == nil {
		a.revokedJTIs = make(map[string]time.Time)
	}
	a.revokedJTIs[token] = time.Now()
	db := a.db
	secret := a.jwtSecret
	a.mu.Unlock()

	exp := time.Now().Add(a.sessionDuration)
	if claims, err := VerifyJWT(token, secret); err == nil && claims != nil {
		if claims.ID != "" {
			a.mu.Lock()
			a.revokedJTIs[claims.ID] = time.Now()
			a.mu.Unlock()
			if db != nil {
				_ = db.RecordRevokedToken(claims.ID, time.Unix(claims.ExpiresAt, 0))
			}
		}
		exp = time.Unix(claims.ExpiresAt, 0)
	}

	if db != nil {
		_ = db.RecordRevokedToken(token, exp)
	}
}

// RevokeUserSessions revokes all sessions for a user, updating user timestamp to reject active JWTs.
func (a *AuthManager) RevokeUserSessions(userID string) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return
	}
	a.mu.Lock()
	if a.revokedJTIs == nil {
		a.revokedJTIs = make(map[string]time.Time)
	}
	for token, session := range a.sessions {
		if session.UserID == userID {
			delete(a.sessions, token)
			a.revokedJTIs[token] = time.Now()
		}
	}
	db := a.db
	a.mu.Unlock()

	if db != nil {
		_ = db.TouchRBACUser(userID)
	}
}

// RevokeAllSessions invalidates all currently active sessions and JWTs.
func (a *AuthManager) RevokeAllSessions() {
	a.mu.Lock()
	a.globalRevocationCutoff = time.Now()
	a.sessions = make(map[string]Session)
	a.revokedJTIs = make(map[string]time.Time)
	a.mu.Unlock()
}

// SessionDurationHours returns the configured session duration in hours.
func (a *AuthManager) SessionDurationHours() int {
	return int(a.sessionDuration / time.Hour)
}

func allPermissions() map[string]bool {
	out := make(map[string]bool, len(PermissionCatalog))
	for key := range PermissionCatalog {
		out[key] = true
	}
	return out
}

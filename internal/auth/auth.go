package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"kestrel/internal/database"
)

const (
	bcryptCost = 12
	// TokenHeaderName is the HTTP header carrying the bearer token.
	TokenHeaderName = "Authorization"
)

// Claims are the JWT payload fields.
type Claims struct {
	UserID             string `json:"uid"`
	Username           string `json:"sub"`
	SessionID          string `json:"sid,omitempty"`
	MustChangePassword bool   `json:"mcp,omitempty"`
	jwt.RegisteredClaims
}

// Service handles authentication operations.
type Service struct {
	db         *database.DB
	jwtSecret  []byte
	sessionTTL time.Duration
}

// New creates an auth service.
func New(db *database.DB, jwtSecret string, sessionHours int) *Service {
	if sessionHours <= 0 {
		sessionHours = 12
	}
	return &Service{
		db:         db,
		jwtSecret:  []byte(jwtSecret),
		sessionTTL: time.Duration(sessionHours) * time.Hour,
	}
}

// HashPassword returns a bcrypt hash of the given plaintext password.
func HashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("hashing password: %w", err)
	}
	return string(b), nil
}

// CheckPassword verifies a plaintext password against its bcrypt hash.
func CheckPassword(password, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// ErrBadCredentials is returned when login credentials are invalid.
var ErrBadCredentials = errors.New("invalid username or password")

// Login validates credentials and returns a signed JWT.
func (s *Service) Login(username, password, clientIP, userAgent string) (string, *database.User, error) {
	user, err := s.db.GetUserByUsername(username)
	if err != nil {
		return "", nil, fmt.Errorf("looking up user: %w", err)
	}
	if user == nil || !CheckPassword(password, user.PasswordHash) {
		return "", nil, ErrBadCredentials
	}
	if !user.IsActive {
		return "", nil, errors.New("account is disabled")
	}

	token, err := s.issueToken(user)
	if err != nil {
		return "", nil, err
	}

	// Persist session for revocation capability.
	tokenHash := hashToken(token)
	now := time.Now().UTC()
	_, err = s.db.Exec(`
		INSERT INTO sessions (id,user_id,token_hash,ip_address,user_agent,created_at,expires_at,last_seen_at)
		VALUES (?,?,?,?,?,?,?,?)`,
		uuid.New().String(), user.ID, tokenHash, clientIP, userAgent,
		now, now.Add(s.sessionTTL), now,
	)
	if err != nil {
		return "", nil, fmt.Errorf("persisting session: %w", err)
	}

	return token, user, nil
}

// Logout invalidates the token stored in the request.
func (s *Service) Logout(token string) error {
	tokenHash := hashToken(token)
	_, err := s.db.Exec(`DELETE FROM sessions WHERE token_hash=?`, tokenHash)
	return err
}

// issueToken creates a signed JWT for the given user.
func (s *Service) issueToken(user *database.User) (string, error) {
	now := time.Now().UTC()
	claims := Claims{
		UserID:             user.ID,
		Username:           user.Username,
		MustChangePassword: user.MustChangePassword,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.sessionTTL)),
			Issuer:    "kestrel",
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString(s.jwtSecret)
}

// ValidateToken parses and validates a JWT, checking revocation.
func (s *Service) ValidateToken(tokenStr string) (*Claims, error) {
	tok, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.jwtSecret, nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := tok.Claims.(*Claims)
	if !ok || !tok.Valid {
		return nil, errors.New("invalid token")
	}

	// Check session revocation.
	tokenHash := hashToken(tokenStr)
	var count int
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM sessions WHERE token_hash=? AND expires_at > ?`,
		tokenHash, time.Now().UTC(),
	).Scan(&count); err != nil || count == 0 {
		return nil, errors.New("session not found or expired")
	}

	// Touch last_seen_at.
	_, _ = s.db.Exec(`UPDATE sessions SET last_seen_at=? WHERE token_hash=?`, time.Now().UTC(), tokenHash)

	return claims, nil
}

// ExtractBearerToken extracts the token from the Authorization header.
func ExtractBearerToken(r *http.Request) string {
	header := r.Header.Get(TokenHeaderName)
	if strings.HasPrefix(header, "Bearer ") {
		return strings.TrimPrefix(header, "Bearer ")
	}
	// Also check query param for WebSocket connections.
	return r.URL.Query().Get("token")
}

// hashToken produces a SHA-256 hex hash of a token for safe DB storage.
func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// PurgeExpiredSessions removes expired sessions from the database.
func (s *Service) PurgeExpiredSessions() error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE expires_at < ?`, time.Now().UTC())
	return err
}

// IssueToken creates a signed JWT for a user ID and optional session ID.
// It does NOT persist a session record — use Login for real auth flows.
// Exposed for use in tests and internal tooling.
func (s *Service) IssueToken(userID, sessionID string) (string, error) {
	now := time.Now().UTC()
	claims := Claims{
		UserID:    userID,
		SessionID: sessionID,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.sessionTTL)),
			Issuer:    "kestrel",
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString(s.jwtSecret)
}

// ValidateTokenInsecure parses a JWT without checking the session DB.
// Use only in tests and internal tooling — NOT in request handlers.
func (s *Service) ValidateTokenInsecure(tokenStr string) (*Claims, error) {
	tok, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return s.jwtSecret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := tok.Claims.(*Claims)
	if !ok || !tok.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}

// NewForTest creates a Service without a database dependency for unit testing.
func NewForTest(jwtSecret string) *Service {
	return &Service{
		jwtSecret:  []byte(jwtSecret),
		sessionTTL: 12 * time.Hour,
	}
}

package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrInvalidToken   = errors.New("invalid token")
	ErrTokenExpired   = errors.New("token expired")
	ErrTokenSignature = errors.New("invalid token signature")
)

// JWTClaims holds standard and custom claims for signed sessions.
type JWTClaims struct {
	Subject            string            `json:"sub"`
	Username           string            `json:"username"`
	DisplayName        string            `json:"displayName,omitempty"`
	MustChangePassword bool              `json:"must_change_password,omitempty"`
	Roles              []string          `json:"roles,omitempty"`
	Permissions        map[string]bool   `json:"permissions,omitempty"`
	PermissionScopes   map[string]string `json:"permission_scopes,omitempty"`
	Scope              string            `json:"scope,omitempty"`
	ID                 string            `json:"jti"`
	IssuedAt           int64             `json:"iat"`
	ExpiresAt          int64             `json:"exp"`
}

// GenerateRandomKey generates n cryptographically random bytes.
func GenerateRandomKey(n int) ([]byte, error) {
	if n <= 0 {
		n = 32
	}
	key := make([]byte, n)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("failed to generate random key: %w", err)
	}
	return key, nil
}

// SignJWT creates an HMAC-SHA256 (HS256) signed JWT.
func SignJWT(claims JWTClaims, secret []byte) (string, error) {
	if len(secret) == 0 {
		return "", errors.New("empty jwt secret")
	}
	headerJSON := []byte(`{"alg":"HS256","typ":"JWT"}`)
	headerEnc := base64.RawURLEncoding.EncodeToString(headerJSON)

	payloadJSON, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("failed to marshal jwt claims: %w", err)
	}
	payloadEnc := base64.RawURLEncoding.EncodeToString(payloadJSON)

	signingInput := headerEnc + "." + payloadEnc
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(signingInput))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return signingInput + "." + signature, nil
}

// VerifyJWT parses and verifies an HMAC-SHA256 (HS256) signed JWT.
func VerifyJWT(tokenString string, secret []byte) (*JWTClaims, error) {
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return nil, ErrInvalidToken
	}

	headerEnc, payloadEnc, sigEnc := parts[0], parts[1], parts[2]

	signingInput := headerEnc + "." + payloadEnc
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(signingInput))
	expectedSig := mac.Sum(nil)

	actualSig, err := base64.RawURLEncoding.DecodeString(sigEnc)
	if err != nil || !hmac.Equal(actualSig, expectedSig) {
		return nil, ErrTokenSignature
	}

	payloadJSON, err := base64.RawURLEncoding.DecodeString(payloadEnc)
	if err != nil {
		return nil, ErrInvalidToken
	}

	var claims JWTClaims
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		return nil, ErrInvalidToken
	}

	if claims.ExpiresAt > 0 && time.Now().Unix() > claims.ExpiresAt {
		return nil, ErrTokenExpired
	}

	return &claims, nil
}


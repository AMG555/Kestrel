package handler

import (
	"golang.org/x/crypto/bcrypt"
)

// hashPasswordBcrypt is the shared bcrypt helper used within the handler package.
func hashPasswordBcrypt(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

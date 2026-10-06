package database

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// User represents a platform user.
type User struct {
	ID                 string    `json:"id"`
	Username           string    `json:"username"`
	PasswordHash       string    `json:"-"`
	Email              string    `json:"email"`
	DisplayName        string    `json:"display_name"`
	MustChangePassword bool      `json:"must_change_password"`
	IsActive           bool      `json:"is_active"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// CreateUser inserts a new user record.
func (db *DB) CreateUser(username, passwordHash, email, displayName string) (*User, error) {
	now := time.Now().UTC()
	u := &User{
		ID:                 uuid.New().String(),
		Username:           username,
		PasswordHash:       passwordHash,
		Email:              email,
		DisplayName:        displayName,
		MustChangePassword: true,
		IsActive:           true,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	_, err := db.Exec(`
		INSERT INTO users (id,username,password_hash,email,display_name,must_change_password,is_active,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		u.ID, u.Username, u.PasswordHash, u.Email, u.DisplayName,
		boolToInt(u.MustChangePassword), boolToInt(u.IsActive),
		u.CreatedAt, u.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("creating user: %w", err)
	}
	return u, nil
}

// GetUserByUsername looks up a user by username.
func (db *DB) GetUserByUsername(username string) (*User, error) {
	return db.scanUser(db.QueryRow(
		`SELECT id,username,password_hash,email,display_name,must_change_password,is_active,created_at,updated_at
		 FROM users WHERE username=? AND is_active=1`, username,
	))
}

// GetUserByID looks up a user by ID.
func (db *DB) GetUserByID(id string) (*User, error) {
	return db.scanUser(db.QueryRow(
		`SELECT id,username,password_hash,email,display_name,must_change_password,is_active,created_at,updated_at
		 FROM users WHERE id=?`, id,
	))
}

// ListUsers returns all users.
func (db *DB) ListUsers() ([]*User, error) {
	rows, err := db.Query(
		`SELECT id,username,password_hash,email,display_name,must_change_password,is_active,created_at,updated_at
		 FROM users ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		u, err := db.scanUserRow(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// UpdateUserPassword updates a user's password hash and clears the forced-change flag.
func (db *DB) UpdateUserPassword(userID, passwordHash string) error {
	_, err := db.Exec(
		`UPDATE users SET password_hash=?, must_change_password=0, updated_at=? WHERE id=?`,
		passwordHash, time.Now().UTC(), userID,
	)
	return err
}

// UpdateUser updates mutable user fields.
func (db *DB) UpdateUser(userID, email, displayName string, isActive bool) error {
	_, err := db.Exec(
		`UPDATE users SET email=?, display_name=?, is_active=?, updated_at=? WHERE id=?`,
		email, displayName, boolToInt(isActive), time.Now().UTC(), userID,
	)
	return err
}

// DeleteUser removes a user by ID.
func (db *DB) DeleteUser(userID string) error {
	_, err := db.Exec(`DELETE FROM users WHERE id=?`, userID)
	return err
}

// CountUsers returns total user count.
func (db *DB) CountUsers() (int, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

// AdminExists returns true if any user exists in the database.
func (db *DB) AdminExists() (bool, error) {
	n, err := db.CountUsers()
	return n > 0, err
}

func (db *DB) scanUser(row *sql.Row) (*User, error) {
	u := &User{}
	var mustChange, isActive int
	err := row.Scan(
		&u.ID, &u.Username, &u.PasswordHash, &u.Email, &u.DisplayName,
		&mustChange, &isActive, &u.CreatedAt, &u.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	u.MustChangePassword = mustChange == 1
	u.IsActive = isActive == 1
	return u, nil
}

func (db *DB) scanUserRow(rows *sql.Rows) (*User, error) {
	u := &User{}
	var mustChange, isActive int
	err := rows.Scan(
		&u.ID, &u.Username, &u.PasswordHash, &u.Email, &u.DisplayName,
		&mustChange, &isActive, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	u.MustChangePassword = mustChange == 1
	u.IsActive = isActive == 1
	return u, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

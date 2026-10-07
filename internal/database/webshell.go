package database

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// WebShellConnRecord represents a stored webshell connection configuration.
type WebShellConnRecord struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	URL       string    `json:"url"`
	Type      string    `json:"type"`
	Password  string    `json:"password"`
	Encoding  string    `json:"encoding"`
	OS        string    `json:"os"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// ListWebshellConnections returns all registered webshell connections.
func (db *DB) ListWebshellConnections() ([]*WebShellConnRecord, error) {
	rows, err := db.Query(`SELECT id, name, url, type, password, encoding, os, status, created_at FROM webshell_connections ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("listing webshell connections: %w", err)
	}
	defer rows.Close()

	var list []*WebShellConnRecord
	for rows.Next() {
		var r WebShellConnRecord
		if err := rows.Scan(&r.ID, &r.Name, &r.URL, &r.Type, &r.Password, &r.Encoding, &r.OS, &r.Status, &r.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, &r)
	}
	return list, nil
}

// CreateWebshellConnection persists a new webshell connection.
func (db *DB) CreateWebshellConnection(r *WebShellConnRecord) (*WebShellConnRecord, error) {
	if r.ID == "" {
		r.ID = uuid.New().String()
	}
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now().UTC()
	}
	_, err := db.Exec(`INSERT INTO webshell_connections (id, name, url, type, password, encoding, os, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.Name, r.URL, r.Type, r.Password, r.Encoding, r.OS, r.Status, r.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting webshell connection: %w", err)
	}
	return r, nil
}

// GetWebshellConnection retrieves a connection by ID.
func (db *DB) GetWebshellConnection(id string) (*WebShellConnRecord, error) {
	var r WebShellConnRecord
	err := db.QueryRow(`SELECT id, name, url, type, password, encoding, os, status, created_at FROM webshell_connections WHERE id = ?`, id).
		Scan(&r.ID, &r.Name, &r.URL, &r.Type, &r.Password, &r.Encoding, &r.OS, &r.Status, &r.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// UpdateWebshellStatus updates status for a connection.
func (db *DB) UpdateWebshellStatus(id, status string) error {
	_, err := db.Exec(`UPDATE webshell_connections SET status = ? WHERE id = ?`, status, id)
	return err
}

// DeleteWebshellConnection deletes a connection by ID.
func (db *DB) DeleteWebshellConnection(id string) error {
	_, err := db.Exec(`DELETE FROM webshell_connections WHERE id = ?`, id)
	return err
}

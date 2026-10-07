package database

import (
	"database/sql"
	"strings"
	"time"
)

// ConversationLastActivity returns the most recent activity time for a conversation; ok=false means the conversation no longer exists.
// Used by storage cleanup to determine whether a directory is orphaned and whether the conversation is still actively in use.
func (db *DB) ConversationLastActivity(id string) (time.Time, bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return time.Time{}, false, nil
	}
	var createdAt, updatedAt string
	err := db.QueryRow(
		"SELECT created_at, updated_at FROM conversations WHERE id = ? LIMIT 1", id,
	).Scan(&createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	created, updated := parseDBTime(createdAt), parseDBTime(updatedAt)
	if created.After(updated) {
		return created, true, nil
	}
	return updated, true, nil
}

// ProjectLastActivity returns the most recent activity time for a project; ok=false means the project no longer exists.
func (db *DB) ProjectLastActivity(id string) (time.Time, bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return time.Time{}, false, nil
	}
	var createdAt, updatedAt string
	err := db.QueryRow(
		"SELECT created_at, updated_at FROM projects WHERE id = ? LIMIT 1", id,
	).Scan(&createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	created, updated := parseDBTime(createdAt), parseDBTime(updatedAt)
	if created.After(updated) {
		return created, true, nil
	}
	return updated, true, nil
}

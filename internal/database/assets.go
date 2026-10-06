package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Asset represents a tracked network/web asset.
type Asset struct {
	ID                 string    `json:"id"`
	ProjectID          string    `json:"project_id,omitempty"`
	DedupKey           string    `json:"dedup_key"`
	Host               string    `json:"host"`
	IP                 string    `json:"ip"`
	Port               int       `json:"port"`
	Domain             string    `json:"domain"`
	Protocol           string    `json:"protocol"`
	Title              string    `json:"title"`
	Service            string    `json:"service"`
	Status             string    `json:"status"`
	OwnerUserID        string    `json:"owner_user_id"`
	Tags               []string  `json:"tags"`
	VulnerabilityCount int       `json:"vulnerability_count"`
	RiskLevel          string    `json:"risk_level"`
	Source             string    `json:"source"`
	FirstSeenAt        time.Time `json:"first_seen_at"`
	LastSeenAt         time.Time `json:"last_seen_at"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// UpsertAsset inserts or updates an asset by its dedup_key.
func (db *DB) UpsertAsset(a *Asset) (*Asset, error) {
	if a.DedupKey == "" {
		a.DedupKey = buildDedupKey(a)
	}
	tagsJSON, _ := json.Marshal(a.Tags)
	now := time.Now().UTC()
	if a.ID == "" {
		a.ID = uuid.New().String()
	}
	if a.FirstSeenAt.IsZero() {
		a.FirstSeenAt = now
	}
	a.LastSeenAt = now
	a.UpdatedAt = now

	_, err := db.Exec(`
		INSERT INTO assets
		  (id,project_id,dedup_key,host,ip,port,domain,protocol,title,service,status,owner_user_id,
		   tags_json,vulnerability_count,risk_level,source,first_seen_at,last_seen_at,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(dedup_key) DO UPDATE SET
		  host=excluded.host, ip=excluded.ip, port=excluded.port,
		  domain=excluded.domain, protocol=excluded.protocol, title=excluded.title,
		  service=excluded.service, status=excluded.status,
		  last_seen_at=excluded.last_seen_at, updated_at=excluded.updated_at`,
		a.ID, nullStr(a.ProjectID), a.DedupKey,
		a.Host, a.IP, a.Port, a.Domain, a.Protocol, a.Title, a.Service,
		a.Status, a.OwnerUserID, string(tagsJSON),
		a.VulnerabilityCount, a.RiskLevel, a.Source,
		a.FirstSeenAt, a.LastSeenAt, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("upserting asset: %w", err)
	}
	return a, nil
}

// GetAssetByID retrieves an asset by ID.
func (db *DB) GetAssetByID(id string) (*Asset, error) {
	return db.scanAsset(db.QueryRow(
		`SELECT id,COALESCE(project_id,''),dedup_key,host,ip,port,domain,protocol,title,service,
		        status,owner_user_id,tags_json,vulnerability_count,risk_level,source,
		        first_seen_at,last_seen_at,created_at,updated_at
		 FROM assets WHERE id=?`, id,
	))
}

// ListAssetsParams filters asset list queries.
type ListAssetsParams struct {
	ProjectID string
	Status    string
	RiskLevel string
	Search    string
	Limit     int
	Offset    int
}

// ListAssets returns assets matching the given filters.
func (db *DB) ListAssets(p ListAssetsParams) ([]*Asset, int, error) {
	if p.Limit <= 0 {
		p.Limit = 50
	}

	where := "WHERE 1=1"
	args := []interface{}{}

	if p.ProjectID != "" {
		where += " AND project_id=?"
		args = append(args, p.ProjectID)
	}
	if p.Status != "" {
		where += " AND status=?"
		args = append(args, p.Status)
	}
	if p.RiskLevel != "" {
		where += " AND risk_level=?"
		args = append(args, p.RiskLevel)
	}
	if p.Search != "" {
		where += " AND (host LIKE ? OR ip LIKE ? OR domain LIKE ?)"
		s := "%" + p.Search + "%"
		args = append(args, s, s, s)
	}

	var total int
	countArgs := make([]interface{}, len(args))
	copy(countArgs, args)
	if err := db.QueryRow("SELECT COUNT(*) FROM assets "+where, countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, p.Limit, p.Offset)
	rows, err := db.Query(`
		SELECT id,COALESCE(project_id,''),dedup_key,host,ip,port,domain,protocol,title,service,
		       status,owner_user_id,tags_json,vulnerability_count,risk_level,source,
		       first_seen_at,last_seen_at,created_at,updated_at
		FROM assets `+where+` ORDER BY last_seen_at DESC LIMIT ? OFFSET ?`, args...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var assets []*Asset
	for rows.Next() {
		a, err := db.scanAssetRow(rows)
		if err != nil {
			return nil, 0, err
		}
		assets = append(assets, a)
	}
	return assets, total, rows.Err()
}

// DeleteAsset removes an asset.
func (db *DB) DeleteAsset(id string) error {
	_, err := db.Exec(`DELETE FROM assets WHERE id=?`, id)
	return err
}

func (db *DB) scanAsset(row *sql.Row) (*Asset, error) {
	a := &Asset{}
	var tagsJSON string
	err := row.Scan(
		&a.ID, &a.ProjectID, &a.DedupKey,
		&a.Host, &a.IP, &a.Port, &a.Domain, &a.Protocol, &a.Title, &a.Service,
		&a.Status, &a.OwnerUserID, &tagsJSON,
		&a.VulnerabilityCount, &a.RiskLevel, &a.Source,
		&a.FirstSeenAt, &a.LastSeenAt, &a.CreatedAt, &a.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(tagsJSON), &a.Tags)
	return a, nil
}

func (db *DB) scanAssetRow(rows *sql.Rows) (*Asset, error) {
	a := &Asset{}
	var tagsJSON string
	err := rows.Scan(
		&a.ID, &a.ProjectID, &a.DedupKey,
		&a.Host, &a.IP, &a.Port, &a.Domain, &a.Protocol, &a.Title, &a.Service,
		&a.Status, &a.OwnerUserID, &tagsJSON,
		&a.VulnerabilityCount, &a.RiskLevel, &a.Source,
		&a.FirstSeenAt, &a.LastSeenAt, &a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(tagsJSON), &a.Tags)
	return a, nil
}

func buildDedupKey(a *Asset) string {
	if a.IP != "" && a.Port > 0 {
		return fmt.Sprintf("ip:%s:%d", a.IP, a.Port)
	}
	if a.Domain != "" && a.Port > 0 {
		return fmt.Sprintf("domain:%s:%d", a.Domain, a.Port)
	}
	if a.Domain != "" {
		return fmt.Sprintf("domain:%s", a.Domain)
	}
	if a.IP != "" {
		return fmt.Sprintf("ip:%s", a.IP)
	}
	return fmt.Sprintf("host:%s", a.Host)
}

func nullStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

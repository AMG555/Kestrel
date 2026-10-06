package database

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Vulnerability represents a tracked security finding.
type Vulnerability struct {
	ID                 string    `json:"id"`
	ProjectID          string    `json:"project_id,omitempty"`
	AssetID            string    `json:"asset_id,omitempty"`
	SessionID          string    `json:"session_id"`
	Title              string    `json:"title"`
	Description        string    `json:"description"`
	Severity           string    `json:"severity"` // critical | high | medium | low | info
	Status             string    `json:"status"`   // open | in_progress | resolved | accepted_risk | false_positive
	VulnType           string    `json:"vuln_type"`
	Target             string    `json:"target"`
	ReproductionSteps  string    `json:"reproduction_steps"`
	Evidence           string    `json:"evidence"`
	Recommendation     string    `json:"recommendation"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// CreateVulnerability inserts a new vulnerability record.
func (db *DB) CreateVulnerability(v *Vulnerability) (*Vulnerability, error) {
	now := time.Now().UTC()
	v.ID = uuid.New().String()
	v.CreatedAt = now
	v.UpdatedAt = now
	if v.Status == "" {
		v.Status = "open"
	}
	if v.Severity == "" {
		v.Severity = "info"
	}

	_, err := db.Exec(`
		INSERT INTO vulnerabilities
		  (id,project_id,asset_id,session_id,title,description,severity,status,vuln_type,
		   target,reproduction_steps,evidence,recommendation,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		v.ID, nullStr(v.ProjectID), nullStr(v.AssetID), v.SessionID,
		v.Title, v.Description, v.Severity, v.Status, v.VulnType,
		v.Target, v.ReproductionSteps, v.Evidence, v.Recommendation,
		v.CreatedAt, v.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("creating vulnerability: %w", err)
	}
	return v, nil
}

// GetVulnerabilityByID retrieves a vulnerability by ID.
func (db *DB) GetVulnerabilityByID(id string) (*Vulnerability, error) {
	return db.scanVuln(db.QueryRow(
		`SELECT id,COALESCE(project_id,''),COALESCE(asset_id,''),session_id,title,description,severity,status,vuln_type,
		        target,reproduction_steps,evidence,recommendation,created_at,updated_at
		 FROM vulnerabilities WHERE id=?`, id,
	))
}

// ListVulnsParams filters vulnerability list queries.
type ListVulnsParams struct {
	ProjectID string
	AssetID   string
	Severity  string
	Status    string
	Limit     int
	Offset    int
}

// ListVulnerabilities returns vulnerabilities matching the given filters.
func (db *DB) ListVulnerabilities(p ListVulnsParams) ([]*Vulnerability, int, error) {
	if p.Limit <= 0 {
		p.Limit = 50
	}

	where := "WHERE 1=1"
	args := []interface{}{}

	if p.ProjectID != "" {
		where += " AND project_id=?"
		args = append(args, p.ProjectID)
	}
	if p.AssetID != "" {
		where += " AND asset_id=?"
		args = append(args, p.AssetID)
	}
	if p.Severity != "" {
		where += " AND severity=?"
		args = append(args, p.Severity)
	}
	if p.Status != "" {
		where += " AND status=?"
		args = append(args, p.Status)
	}

	var total int
	countArgs := make([]interface{}, len(args))
	copy(countArgs, args)
	if err := db.QueryRow("SELECT COUNT(*) FROM vulnerabilities "+where, countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, p.Limit, p.Offset)
	rows, err := db.Query(`
		SELECT id,COALESCE(project_id,''),COALESCE(asset_id,''),session_id,title,description,severity,status,vuln_type,
		       target,reproduction_steps,evidence,recommendation,created_at,updated_at
		FROM vulnerabilities `+where+` ORDER BY
		  CASE severity WHEN 'critical' THEN 1 WHEN 'high' THEN 2 WHEN 'medium' THEN 3 WHEN 'low' THEN 4 ELSE 5 END,
		  created_at DESC LIMIT ? OFFSET ?`, args...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var vulns []*Vulnerability
	for rows.Next() {
		v := &Vulnerability{}
		err := rows.Scan(
			&v.ID, &v.ProjectID, &v.AssetID, &v.SessionID,
			&v.Title, &v.Description, &v.Severity, &v.Status, &v.VulnType,
			&v.Target, &v.ReproductionSteps, &v.Evidence, &v.Recommendation,
			&v.CreatedAt, &v.UpdatedAt,
		)
		if err != nil {
			return nil, 0, err
		}
		vulns = append(vulns, v)
	}
	return vulns, total, rows.Err()
}

// UpdateVulnerability updates mutable vulnerability fields.
func (db *DB) UpdateVulnerability(id, severity, status, description, recommendation string) error {
	_, err := db.Exec(`
		UPDATE vulnerabilities SET severity=?,status=?,description=?,recommendation=?,updated_at=?
		WHERE id=?`,
		severity, status, description, recommendation, time.Now().UTC(), id,
	)
	return err
}

// DeleteVulnerability removes a vulnerability.
func (db *DB) DeleteVulnerability(id string) error {
	_, err := db.Exec(`DELETE FROM vulnerabilities WHERE id=?`, id)
	return err
}

// VulnerabilitySummary holds severity breakdown stats.
type VulnerabilitySummary struct {
	Total    int `json:"total"`
	Critical int `json:"critical"`
	High     int `json:"high"`
	Medium   int `json:"medium"`
	Low      int `json:"low"`
	Info     int `json:"info"`
	Open     int `json:"open"`
	Resolved int `json:"resolved"`
}

// VulnerabilitySummaryByProject returns severity breakdown for a project.
func (db *DB) VulnerabilitySummaryByProject(projectID string) (*VulnerabilitySummary, error) {
	rows, err := db.Query(`
		SELECT severity, status, COUNT(*) FROM vulnerabilities
		WHERE project_id=? GROUP BY severity, status`, projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	s := &VulnerabilitySummary{}
	for rows.Next() {
		var sev, status string
		var count int
		if err := rows.Scan(&sev, &status, &count); err != nil {
			return nil, err
		}
		s.Total += count
		switch sev {
		case "critical":
			s.Critical += count
		case "high":
			s.High += count
		case "medium":
			s.Medium += count
		case "low":
			s.Low += count
		default:
			s.Info += count
		}
		if status == "open" || status == "in_progress" {
			s.Open += count
		}
		if status == "resolved" {
			s.Resolved += count
		}
	}
	return s, rows.Err()
}

func (db *DB) scanVuln(row *sql.Row) (*Vulnerability, error) {
	v := &Vulnerability{}
	err := row.Scan(
		&v.ID, &v.ProjectID, &v.AssetID, &v.SessionID,
		&v.Title, &v.Description, &v.Severity, &v.Status, &v.VulnType,
		&v.Target, &v.ReproductionSteps, &v.Evidence, &v.Recommendation,
		&v.CreatedAt, &v.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return v, nil
}

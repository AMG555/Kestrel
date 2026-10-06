package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Project groups agent sessions, assets, and vulnerabilities under a shared scope.
type Project struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	ScopeJSON   string    `json:"-"`
	Scope       []string  `json:"scope"`
	Status      string    `json:"status"` // active | archived | closed
	OwnerUserID string    `json:"owner_user_id"`
	Pinned      bool      `json:"pinned"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// CreateProject inserts a new project.
func (db *DB) CreateProject(name, description, ownerUserID string, scope []string) (*Project, error) {
	now := time.Now().UTC()
	scopeJSON, _ := json.Marshal(scope)
	p := &Project{
		ID:          uuid.New().String(),
		Name:        name,
		Description: description,
		Scope:       scope,
		Status:      "active",
		OwnerUserID: ownerUserID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	_, err := db.Exec(`
		INSERT INTO projects (id,name,description,scope_json,status,owner_user_id,pinned,created_at,updated_at)
		VALUES (?,?,?,?,?,?,0,?,?)`,
		p.ID, p.Name, p.Description, string(scopeJSON), p.Status, p.OwnerUserID, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("creating project: %w", err)
	}
	return p, nil
}

// GetProjectByID retrieves a single project.
func (db *DB) GetProjectByID(id string) (*Project, error) {
	return db.scanProject(db.QueryRow(
		`SELECT id,name,description,scope_json,status,owner_user_id,pinned,created_at,updated_at
		 FROM projects WHERE id=?`, id,
	))
}

// ListProjectsParams holds list filters.
type ListProjectsParams struct {
	Status  string
	Limit   int
	Offset  int
}

// ListProjects returns projects, pinned-first then newest.
func (db *DB) ListProjects(p ListProjectsParams) ([]*Project, int, error) {
	if p.Limit <= 0 {
		p.Limit = 50
	}
	where := "WHERE 1=1"
	args := []interface{}{}
	if p.Status != "" {
		where += " AND status=?"
		args = append(args, p.Status)
	}

	var total int
	countArgs := make([]interface{}, len(args))
	copy(countArgs, args)
	if err := db.QueryRow("SELECT COUNT(*) FROM projects "+where, countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, p.Limit, p.Offset)
	rows, err := db.Query(`
		SELECT id,name,description,scope_json,status,owner_user_id,pinned,created_at,updated_at
		FROM projects `+where+` ORDER BY pinned DESC, updated_at DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var projects []*Project
	for rows.Next() {
		proj, err := db.scanProjectRow(rows)
		if err != nil {
			return nil, 0, err
		}
		projects = append(projects, proj)
	}
	return projects, total, rows.Err()
}

// UpdateProject updates mutable project fields.
func (db *DB) UpdateProject(id, name, description, status string, scope []string, pinned bool) error {
	scopeJSON, _ := json.Marshal(scope)
	_, err := db.Exec(
		`UPDATE projects SET name=?,description=?,scope_json=?,status=?,pinned=?,updated_at=? WHERE id=?`,
		name, description, string(scopeJSON), status, boolToInt(pinned), time.Now().UTC(), id,
	)
	return err
}

// DeleteProject removes a project (cascades to facts, linked sessions via SET NULL).
func (db *DB) DeleteProject(id string) error {
	_, err := db.Exec(`DELETE FROM projects WHERE id=?`, id)
	return err
}

// ProjectFact is a key-value fact stored on the project blackboard.
type ProjectFact struct {
	ID           string    `json:"id"`
	ProjectID    string    `json:"project_id"`
	FactKey      string    `json:"fact_key"`
	Category     string    `json:"category"` // note | finding | target | credential | recon | chain
	Summary      string    `json:"summary"`
	Body         string    `json:"body,omitempty"`
	Confidence   string    `json:"confidence"` // confirmed | tentative | deprecated
	SourceSessID string    `json:"source_session_id,omitempty"`
	Pinned       bool      `json:"pinned"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// UpsertProjectFact inserts or updates a blackboard fact by (project_id, fact_key).
func (db *DB) UpsertProjectFact(f *ProjectFact) (*ProjectFact, error) {
	now := time.Now().UTC()
	if f.ID == "" {
		f.ID = uuid.New().String()
	}
	f.UpdatedAt = now
	if f.CreatedAt.IsZero() {
		f.CreatedAt = now
	}
	_, err := db.Exec(`
		INSERT INTO project_facts
		  (id,project_id,fact_key,category,summary,body,confidence,source_session_id,pinned,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(project_id,fact_key) DO UPDATE SET
		  category=excluded.category, summary=excluded.summary, body=excluded.body,
		  confidence=excluded.confidence, source_session_id=excluded.source_session_id,
		  pinned=excluded.pinned, updated_at=excluded.updated_at`,
		f.ID, f.ProjectID, f.FactKey, f.Category, f.Summary, f.Body, f.Confidence,
		nullStr(f.SourceSessID), boolToInt(f.Pinned), f.CreatedAt, f.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("upserting project fact: %w", err)
	}
	return f, nil
}

// ListProjectFacts returns all facts for a project.
func (db *DB) ListProjectFacts(projectID string) ([]*ProjectFact, error) {
	rows, err := db.Query(`
		SELECT id,project_id,fact_key,category,summary,COALESCE(body,''),confidence,
		       COALESCE(source_session_id,''),pinned,created_at,updated_at
		FROM project_facts WHERE project_id=?
		ORDER BY pinned DESC, updated_at DESC`, projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var facts []*ProjectFact
	for rows.Next() {
		f := &ProjectFact{}
		var pinned int
		if err := rows.Scan(
			&f.ID, &f.ProjectID, &f.FactKey, &f.Category, &f.Summary, &f.Body,
			&f.Confidence, &f.SourceSessID, &pinned, &f.CreatedAt, &f.UpdatedAt,
		); err != nil {
			return nil, err
		}
		f.Pinned = pinned == 1
		facts = append(facts, f)
	}
	return facts, rows.Err()
}

// DeleteProjectFact removes a fact by ID.
func (db *DB) DeleteProjectFact(id string) error {
	_, err := db.Exec(`DELETE FROM project_facts WHERE id=?`, id)
	return err
}

// ProjectStats holds high-level project statistics.
type ProjectStats struct {
	AssetCount    int `json:"asset_count"`
	VulnCount     int `json:"vuln_count"`
	SessionCount  int `json:"session_count"`
	FactCount     int `json:"fact_count"`
	ToolExecCount int `json:"tool_exec_count"`
}

// GetProjectStats returns statistics for a project.
func (db *DB) GetProjectStats(projectID string) (*ProjectStats, error) {
	s := &ProjectStats{}
	_ = db.QueryRow(`SELECT COUNT(*) FROM assets WHERE project_id=?`, projectID).Scan(&s.AssetCount)
	_ = db.QueryRow(`SELECT COUNT(*) FROM vulnerabilities WHERE project_id=?`, projectID).Scan(&s.VulnCount)
	_ = db.QueryRow(`SELECT COUNT(*) FROM agent_sessions WHERE project_id=?`, projectID).Scan(&s.SessionCount)
	_ = db.QueryRow(`SELECT COUNT(*) FROM project_facts WHERE project_id=?`, projectID).Scan(&s.FactCount)
	_ = db.QueryRow(`SELECT COUNT(*) FROM tool_executions te JOIN agent_sessions s ON s.id=te.session_id WHERE s.project_id=?`, projectID).Scan(&s.ToolExecCount)
	return s, nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

func (db *DB) scanProject(row *sql.Row) (*Project, error) {
	p := &Project{}
	var scopeJSON string
	var pinned int
	err := row.Scan(&p.ID, &p.Name, &p.Description, &scopeJSON, &p.Status, &p.OwnerUserID, &pinned, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(scopeJSON), &p.Scope)
	p.Pinned = pinned == 1
	return p, nil
}

func (db *DB) scanProjectRow(rows *sql.Rows) (*Project, error) {
	p := &Project{}
	var scopeJSON string
	var pinned int
	err := rows.Scan(&p.ID, &p.Name, &p.Description, &scopeJSON, &p.Status, &p.OwnerUserID, &pinned, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(scopeJSON), &p.Scope)
	p.Pinned = pinned == 1
	return p, nil
}

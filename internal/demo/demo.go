package demo

import (
	"fmt"
	"strings"
	"time"

	"kestrel/internal/database"
	"kestrel/internal/report"
	"kestrel/internal/security"
)

// DemoSetup holds credentials and links generated for the demo run.
type DemoSetup struct {
	AdminUsername  string
	AdminPassword  string
	AdminToken     string
	DirectLoginURL string
	ProjectID      string
	ProjectName    string
	ShareToken     string
	ShareURL       string
	BadgeURL       string
	TargetURL      string
}

// SeedDemoProject seeds the database with a pre-configured demo penetration test scenario.
func SeedDemoProject(db *database.DB, jwtSecret []byte, baseURL string) (*DemoSetup, error) {
	if len(jwtSecret) == 0 {
		jwtSecret = []byte("kestrel-demo-jwt-secret-key-32chars!!")
	}

	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}

	adminUser := "admin"
	adminPass := "kestrel-demo-admin"

	// 1. Ensure admin user exists
	user, err := db.GetRBACUserByUsername(adminUser)
	if err != nil || user == nil {
		hash, err := security.HashPassword(adminPass)
		if err != nil {
			return nil, fmt.Errorf("failed to hash demo password: %w", err)
		}
		user, err = db.CreateRBACUser(adminUser, "Kestrel Demo Administrator", hash, true, []string{"admin"})
		if err != nil {
			// If already created concurrently, retrieve it
			user, _ = db.GetRBACUserByUsername(adminUser)
		}
	}
	if user != nil {
		_ = db.SetRBACUserMustChangePassword(user.ID, true)
	}

	// 2. Mint admin JWT token
	claims := security.JWTClaims{
		Subject:            user.ID,
		Username:           user.Username,
		DisplayName:        user.DisplayName,
		MustChangePassword: true,
		Roles:              []string{"admin"},
		Scope:              "all",
		IssuedAt:           time.Now().Unix() + 1,
		ExpiresAt:          time.Now().Add(24 * time.Hour * 7).Unix(),
	}
	adminToken, err := security.SignJWT(claims, jwtSecret)
	if err != nil {
		return nil, fmt.Errorf("failed to mint admin jwt: %w", err)
	}

	// 3. Ensure demo project exists
	projectID := "proj-demo-juiceshop"
	targetURL := "http://localhost:3000"

	proj, err := db.GetProject(projectID)
	if err != nil || proj == nil {
		p := &database.Project{
			ID:          projectID,
			Name:        "Demo Target — OWASP Juice Shop",
			Description: "Automated agentic penetration test scenario against OWASP Juice Shop target instance",
			ScopeJSON:   `{"in_scope":["localhost:3000","127.0.0.1:3000","http://juiceshop:3000"],"out_of_scope":["*.production.local"]}`,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		if _, err := db.CreateProject(p); err != nil {
			return nil, fmt.Errorf("failed to create demo project: %w", err)
		}
	}

	// 4. Seed sample vulnerabilities
	findings := []*database.Vulnerability{
		{
			ID:             "vuln-demo-sqli",
			ProjectID:      projectID,
			Title:          "SQL Injection in Product Search API",
			Description:    "Unsanitized user input in query parameter 'q' directly concatenated into SQLite query in search endpoint.",
			Severity:       "critical",
			Status:         "confirmed",
			Type:           "CWE-89",
			Target:         targetURL + "/rest/products/search?q='",
			Evidence:       "HTTP 500: SQLITE_ERROR: unrecognized token: \"'''\"",
			Impact:         "Complete exfiltration of database contents, user credentials, and password hashes.",
			Recommendation: "Use parameterized queries or ORM sanitization.",
			CreatedAt:      time.Now().Add(-2 * time.Hour),
		},
		{
			ID:             "vuln-demo-bola",
			ProjectID:      projectID,
			Title:          "Broken Object Level Authorization (BOLA) in Basket Retrieval",
			Description:    "Users can view and alter any customer's shopping basket by tampering with the basketId path parameter.",
			Severity:       "high",
			Status:         "open",
			Type:           "CWE-639",
			Target:         targetURL + "/rest/basket/2",
			Evidence:       "GET /rest/basket/2 returns basket details of user ID 2 without authorization check.",
			Impact:         "Unauthorized access to sensitive personal data and order details of other users.",
			Recommendation: "Enforce strict session-to-resource ownership checks in the API gateway.",
			CreatedAt:      time.Now().Add(-1 * time.Hour),
		},
		{
			ID:             "vuln-demo-swagger",
			ProjectID:      projectID,
			Title:          "Exposed Interactive Swagger / OpenAPI Specification",
			Description:    "Internal Swagger UI documentation exposed without authentication on public endpoint.",
			Severity:       "low",
			Status:         "fixed",
			Type:           "CWE-200",
			Target:         targetURL + "/api-docs/",
			Evidence:       "Interactive API explorer accessible anonymously, listing all administrative endpoints.",
			Impact:         "Facilitates reconnaissance and attack surface mapping for unauthenticated actors.",
			Recommendation: "Restrict /api-docs to authenticated administrative networks.",
			CreatedAt:      time.Now().Add(-30 * time.Minute),
		},
	}

	for _, f := range findings {
		existing, _ := db.GetVulnerability(f.ID)
		if existing == nil {
			_, _ = db.CreateVulnerability(f)
		}
	}

	// 5. Generate share permalink
	shareExpires := time.Now().Add(30 * 24 * time.Hour)
	shareToken, tokenHash, err := report.CreateShareToken(projectID, shareExpires, jwtSecret)
	if err == nil {
		_ = db.CreateReportShare(&database.ReportShare{
			ProjectID: projectID,
			TokenHash: tokenHash,
			ExpiresAt: shareExpires,
			CreatedAt: time.Now(),
		})
	}

	shareURL := fmt.Sprintf("%s/r/%s", baseURL, shareToken)
	badgeURL := fmt.Sprintf("%s/badge/%s?token=%s", baseURL, projectID, shareToken)
	directLoginURL := fmt.Sprintf("%s/?token=%s", baseURL, adminToken)

	return &DemoSetup{
		AdminUsername:  adminUser,
		AdminPassword:  adminPass,
		AdminToken:     adminToken,
		DirectLoginURL: directLoginURL,
		ProjectID:      projectID,
		ProjectName:    "Demo Target — OWASP Juice Shop",
		ShareToken:     shareToken,
		ShareURL:       shareURL,
		BadgeURL:       badgeURL,
		TargetURL:      targetURL,
	}, nil
}

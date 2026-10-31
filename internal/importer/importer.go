package importer

import (
	"fmt"
	"strings"
	"time"

	"kestrel/internal/database"
)

// ImportData imports external scan data into the database under projectID and records an audit log.
func ImportData(db *database.DB, data []byte, format, projectID, ownerUserID string) (*ImportResult, error) {
	if db == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	projectID = strings.TrimSpace(projectID)
	if projectID != "" {
		if _, err := db.GetProject(projectID); err != nil {
			return nil, fmt.Errorf("project %q not found: %w", projectID, err)
		}
	}

	detectedFormat := format
	if detectedFormat == "" {
		detectedFormat = DetectFormat(data)
	}
	detectedFormat = strings.ToLower(strings.TrimSpace(detectedFormat))
	if detectedFormat == "" {
		return nil, fmt.Errorf("unable to detect format of input data; specify format explicitly (nmap, nuclei, burp)")
	}

	var assets []*database.Asset
	var vulns []*database.Vulnerability
	var err error

	switch detectedFormat {
	case FormatNmap:
		assets, vulns, err = ParseNmap(data, projectID)
	case FormatNuclei:
		assets, vulns, err = ParseNuclei(data, projectID)
	case FormatBurp:
		assets, vulns, err = ParseBurp(data, projectID)
	default:
		return nil, fmt.Errorf("unsupported import format %q (supported: nmap, nuclei, burp)", detectedFormat)
	}

	if err != nil {
		return nil, err
	}

	res := &ImportResult{
		Format: detectedFormat,
	}

	// 1. Upsert Assets
	if len(assets) > 0 {
		assetRes, err := db.UpsertAssets(assets, ownerUserID, true)
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("asset upsert error: %v", err))
		} else {
			res.AssetsCreated = assetRes.Created
			res.AssetsUpdated = assetRes.Updated
			res.AssetsSkipped = assetRes.Skipped
		}
	}

	// 2. Insert Vulnerabilities
	for _, v := range vulns {
		if v == nil {
			res.VulnsSkipped++
			continue
		}
		created, err := db.CreateVulnerability(v)
		if err != nil {
			res.VulnsSkipped++
			res.Errors = append(res.Errors, fmt.Sprintf("vuln %q error: %v", v.Title, err))
		} else {
			res.VulnsCreated++
			if ownerUserID != "" {
				_ = db.SetResourceOwner("vulnerability", created.ID, ownerUserID)
				_ = db.AssignResourceToUser(ownerUserID, "vulnerability", created.ID)
			}
		}
	}

	// 3. Append to immutable Audit Log
	auditActor := ownerUserID
	if auditActor == "" {
		auditActor = "system"
	}
	_ = db.AppendAuditLog(&database.AuditLog{
		ID:           fmt.Sprintf("audit-import-%d", time.Now().UnixNano()),
		CreatedAt:    time.Now().UTC(),
		Level:        "info",
		Category:     "workbench",
		Action:       "import_scan",
		Result:       "success",
		Actor:        auditActor,
		ResourceType: "project",
		ResourceID:   projectID,
		Message:      fmt.Sprintf("Imported %s scan: %d assets created, %d vulns created", detectedFormat, res.AssetsCreated, res.VulnsCreated),
	})

	return res, nil
}

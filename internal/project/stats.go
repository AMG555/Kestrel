package project

import "kestrel/internal/database"

// GetProjectStats aggregates project statistics (including count of sparse facts needing completion).
func GetProjectStats(db *database.DB, projectID string) (*database.ProjectStats, error) {
	stats, err := db.GetProjectStatsCounts(projectID)
	if err != nil {
		return nil, err
	}
	rows, err := db.ListProjectFactsForSparseCheck(projectID)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		if IsSparseFactBody(r.Category, r.FactKey, r.Body) {
			stats.SparseFactCount++
		}
	}
	return stats, nil
}

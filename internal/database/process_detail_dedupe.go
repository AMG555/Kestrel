package database

import (
	"fmt"
	"strings"
)

// DedupeConsecutiveProcessDetails removes consecutive process detail entries that are semantically identical.
// Uses the raw JSON from the DB data column as a fingerprint to avoid instability from map key ordering during serialization.
func DedupeConsecutiveProcessDetails(rows []ProcessDetail) []ProcessDetail {
	if len(rows) < 2 {
		return rows
	}
	out := make([]ProcessDetail, 0, len(rows))
	var lastKey string
	for _, d := range rows {
		key := processDetailRowKey(d)
		if len(out) > 0 && key != "" && key == lastKey {
			continue
		}
		out = append(out, d)
		lastKey = key
	}
	return out
}

func processDetailRowKey(d ProcessDetail) string {
	return fmt.Sprintf("%s\x00%s\x00%s", d.EventType, strings.TrimSpace(d.Message), d.Data)
}

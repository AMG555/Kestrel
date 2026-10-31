package database

import (
	"net/url"
	"strings"
	"testing"
)

func TestBuildSQLiteDSN(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		wantPath string
	}{
		{
			name:     "plain file path",
			input:    "data/conversations.db",
			wantPath: "data/conversations.db",
		},
		{
			name:     "file with existing query params",
			input:    "data/conversations.db?cache=shared&mode=rwc",
			wantPath: "data/conversations.db",
		},
		{
			name:     "file URI",
			input:    "file:data/conversations.db?mode=rwc",
			wantPath: "file:data/conversations.db",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dsn := buildSQLiteDSN(tc.input)
			if !strings.Contains(dsn, "_journal_mode=WAL") {
				t.Fatalf("expected _journal_mode=WAL in %q", dsn)
			}
			if !strings.Contains(dsn, "_foreign_keys=1") {
				t.Fatalf("expected _foreign_keys=1 in %q", dsn)
			}
			if !strings.Contains(dsn, "_busy_timeout=5000") {
				t.Fatalf("expected _busy_timeout=5000 in %q", dsn)
			}
			if !strings.Contains(dsn, "_synchronous=NORMAL") {
				t.Fatalf("expected _synchronous=NORMAL in %q", dsn)
			}

			parts := strings.SplitN(dsn, "?", 2)
			if len(parts) != 2 {
				t.Fatalf("expected query string in %q", dsn)
			}
			vals, err := url.ParseQuery(parts[1])
			if err != nil {
				t.Fatalf("failed to parse query: %v", err)
			}
			if vals.Get("_journal_mode") != "WAL" {
				t.Fatalf("unexpected journal_mode: %q", vals.Get("_journal_mode"))
			}
		})
	}
}


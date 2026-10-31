package doctor

import (
	"bytes"
	"context"
	"testing"
	"time"

	"kestrel/internal/config"
)

func TestRunDiagnostics(t *testing.T) {
	cfg := &config.Config{}
	cfg.Database.Path = t.TempDir() + "/test.db"
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = 64321
	cfg.Auth.JWTSecret = "very-long-secret-key-that-is-at-least-32-chars-long"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rep := RunDiagnostics(ctx, cfg)
	if rep == nil {
		t.Fatal("expected non-nil report")
	}
	if len(rep.Checks) < 6 {
		t.Fatalf("expected at least 6 checks, got %d", len(rep.Checks))
	}

	var buf bytes.Buffer
	PrintReport(&buf, rep)
	out := buf.String()
	if len(out) == 0 {
		t.Fatal("expected non-empty formatted report")
	}
	t.Logf("Doctor output:\n%s", out)
}

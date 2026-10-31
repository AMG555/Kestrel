package eval

import (
	"bytes"
	"context"
	"testing"
)

func TestRunEvalSuite(t *testing.T) {
	rep := RunEvalSuite(context.Background(), "juice-shop-benchmark")
	if rep == nil {
		t.Fatal("expected non-nil suite report")
	}
	if rep.TotalCases != 5 {
		t.Fatalf("expected 5 test cases, got %d", rep.TotalCases)
	}
	if rep.Precision != 1.0 {
		t.Errorf("expected 100%% precision on golden suite, got %.2f", rep.Precision)
	}
	if rep.Recall != 1.0 {
		t.Errorf("expected 100%% recall on golden suite, got %.2f", rep.Recall)
	}
	if rep.F1Score != 1.0 {
		t.Errorf("expected 1.0 F1 score on golden suite, got %.2f", rep.F1Score)
	}

	var buf bytes.Buffer
	PrintEvalReport(&buf, rep)
	out := buf.String()
	if len(out) == 0 {
		t.Fatal("expected non-empty formatted eval output")
	}

	md := rep.ToMarkdownTable()
	if len(md) == 0 {
		t.Fatal("expected non-empty markdown table")
	}
	t.Logf("Benchmark Output:\n%s", out)
	t.Logf("Markdown:\n%s", md)
}

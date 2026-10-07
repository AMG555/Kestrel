package multiagent

import (
	"strings"
	"testing"
)

// The Eino execute deduplication branch's EOF flush must compute the tail based on mainAssistantBuf;
// incorrectly using TrimSpace(mainAssistantBuf) causes a mismatch with the already-pushed prefix
// at whitespace boundaries, making normalize take the concatenation path and duplicate text.
func TestNormalizeStreamingDelta_eofTailUsesRawBufNotTrim(t *testing.T) {
	wireAccum := "phrase "
	rawFull := "phrase \n"
	_, tail := normalizeStreamingDelta(wireAccum, rawFull)
	if want := "\n"; tail != want {
		t.Fatalf("tail=%q want %q", tail, want)
	}

	nextWrong, badTail := normalizeStreamingDelta(wireAccum, strings.TrimSpace(rawFull))
	if badTail != "phrase" || nextWrong != "phrase phrase" {
		t.Fatalf("trimmed full vs wire prefix mismatch should concat-append; got next=%q badTail=%q", nextWrong, badTail)
	}
}

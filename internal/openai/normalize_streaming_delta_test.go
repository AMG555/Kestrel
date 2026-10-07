package openai

import "testing"

func TestNormalizeStreamingDelta_RepeatedCharBoundary(t *testing.T) {
	// streaming split at repeated digit boundary: "43" first char must not be incorrectly merged with "194" last char.
	cur, d := normalizeStreamingDelta("https://x:194", "43")
	if want := "https://x:19443"; cur != want {
		t.Fatalf("next: want %q got %q", want, cur)
	}
	if d != "43" {
		t.Fatalf("delta: want %q got %q", "43", d)
	}
}

func TestNormalizeStreamingDelta_CumulativePrefix(t *testing.T) {
	cur, d := normalizeStreamingDelta("héllo", "héllo wörld")
	if cur != "héllo wörld" || d != " wörld" {
		t.Fatalf("got cur=%q d=%q", cur, d)
	}
}

func TestNormalizeStreamingDelta_FullRetransmit(t *testing.T) {
	cur, d := normalizeStreamingDelta("héllo", "héllo")
	if d != "" || cur != "héllo" {
		t.Fatalf("got cur=%q d=%q", cur, d)
	}
}

func TestNormalizeStreamingDelta_SingleRuneRepeated(t *testing.T) {
	cur, d := normalizeStreamingDelta("é", "é")
	if want := "éé"; cur != want {
		t.Fatalf("next: want %q got %q", want, cur)
	}
	if d != "é" {
		t.Fatalf("delta: want %q got %q", "é", d)
	}
	cur, d = normalizeStreamingDelta("4", "4")
	if want := "44"; cur != want {
		t.Fatalf("next: want %q got %q", want, cur)
	}
	if d != "4" {
		t.Fatalf("delta: want %q got %q", "4", d)
	}
}

func TestNormalizeStreamingDelta_CumulativeExtendsNumber(t *testing.T) {
	// after buffering "194", receives cumulative string "19443" (note: "1943" is not a prefix of "19443"; cannot rely on a miswritten intermediate HasPrefix check).
	cur, d := normalizeStreamingDelta("194", "19443")
	if want := "19443"; cur != want {
		t.Fatalf("next: want %q got %q", want, cur)
	}
	if d != "43" {
		t.Fatalf("delta: want %q got %q", "43", d)
	}
}

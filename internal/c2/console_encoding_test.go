package c2

import (
	"encoding/base64"
	"testing"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

func mustGBK(t *testing.T, s string) []byte {
	t.Helper()
	out, _, err := transform.Bytes(simplifiedchinese.GBK.NewEncoder(), []byte(s))
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestNormalizeConsoleOutput_WindowsGBK(t *testing.T) {
	raw := mustGBK(t, "Chinesetest")
	got := NormalizeConsoleOutput(raw, "windows")
	if got != "Chinesetest" {
		t.Fatalf("got %q want Chinesetest", got)
	}
}

func TestNormalizeConsoleOutput_UTF8Passthrough(t *testing.T) {
	// Use a UTF-8 multi-byte sequence to verify that non-ASCII content passes through unchanged.
	raw := []byte("hello caf\xc3\xa9") // "hello café" in UTF-8
	got := NormalizeConsoleOutput(raw, "linux")
	if got != "hello café" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveTaskResultText_PrefersB64(t *testing.T) {
	raw := mustGBK(t, "purchase-order")
	b64 := base64.StdEncoding.EncodeToString(raw)
	got := ResolveTaskResultText("", b64, "windows")
	if got != "purchase-order" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveTaskResultText_PlainFallback(t *testing.T) {
	raw := mustGBK(t, "test")
	got := ResolveTaskResultText(string(raw), "", "windows")
	if got != "test" {
		t.Fatalf("got %q", got)
	}
}

package handler

import (
	"testing"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// mustEncode encodes a UTF-8 string using the specified encoding, returning raw bytes for constructing test inputs.
func mustEncode(t *testing.T, s string, enc string) []byte {
	t.Helper()
	var tr transform.Transformer
	switch enc {
	case "gbk":
		tr = simplifiedchinese.GBK.NewEncoder()
	case "gb18030":
		tr = simplifiedchinese.GB18030.NewEncoder()
	default:
		t.Fatalf("unsupported test encoding: %s", enc)
	}
	out, _, err := transform.Bytes(tr, []byte(s))
	if err != nil {
		t.Fatalf("mustEncode(%s) failed: %v", enc, err)
	}
	return out
}

func TestNormalizeWebshellEncoding(t *testing.T) {
	cases := map[string]string{
		"":         "auto",
		"   ":      "auto",
		"auto":     "auto",
		"AUTO":     "auto",
		"utf-8":    "utf-8",
		"UTF-8":    "utf-8",
		"utf8":     "utf-8",
		"gbk":      "gbk",
		"GBK":      "gbk",
		"gb18030":  "gb18030",
		"big5":     "auto", // unsupported, falls back to auto
		"anything": "auto",
	}
	for in, want := range cases {
		if got := normalizeWebshellEncoding(in); got != want {
			t.Errorf("normalizeWebshellEncoding(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDecodeWebshellOutput_AutoDetectsGBK(t *testing.T) {
	// simulate GBK byte stream from a Windows Chinese cmd output
	want := "username                        SID                                            type"
	raw := mustEncode(t, want, "gbk")

	// auto mode: after UTF-8 validation fails, should fall back to GB18030 decoding
	got := decodeWebshellOutput(raw, "auto")
	if got != want {
		t.Errorf("decodeWebshellOutput(auto) = %q, want %q", got, want)
	}

	// explicit GBK mode: should also decode correctly
	got = decodeWebshellOutput(raw, "gbk")
	if got != want {
		t.Errorf("decodeWebshellOutput(gbk) = %q, want %q", got, want)
	}

	// explicit GB18030 mode: GBK is a subset of GB18030, should also decode correctly
	got = decodeWebshellOutput(raw, "gb18030")
	if got != want {
		t.Errorf("decodeWebshellOutput(gb18030) = %q, want %q", got, want)
	}
}

func TestDecodeWebshellOutput_PassthroughUTF8(t *testing.T) {
	// already valid UTF-8 Chinese string, all modes should return the original string unchanged
	want := "hello 世界"
	for _, enc := range []string{"", "auto", "utf-8"} {
		if got := decodeWebshellOutput([]byte(want), enc); got != want {
			t.Errorf("decodeWebshellOutput(%q) passthrough = %q, want %q", enc, got, want)
		}
	}
}

func TestDecodeWebshellOutput_ASCIIStable(t *testing.T) {
	// pure ASCII must remain unchanged in any encoding mode
	want := "whoami\nAdministrator\n"
	for _, enc := range []string{"", "auto", "utf-8", "gbk", "gb18030"} {
		if got := decodeWebshellOutput([]byte(want), enc); got != want {
			t.Errorf("decodeWebshellOutput(%q) ASCII = %q, want %q", enc, got, want)
		}
	}
}

func TestDecodeWebshellOutput_EmptyInput(t *testing.T) {
	// nil/empty input should return empty string without extra allocation
	if got := decodeWebshellOutput(nil, "gbk"); got != "" {
		t.Errorf("decodeWebshellOutput(nil) = %q, want empty", got)
	}
	if got := decodeWebshellOutput([]byte{}, "auto"); got != "" {
		t.Errorf("decodeWebshellOutput([]) = %q, want empty", got)
	}
}

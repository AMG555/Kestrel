package handler

import (
	"strings"
	"testing"

	"kestrel/internal/database"
)

func TestBuildWebshellAssistantContext_WindowsExplicit(t *testing.T) {
	conn := &database.WebShellConnection{
		ID:       "ws_win01",
		Remark:   "IIS Windows target",
		URL:      "http://example.com/shell.php",
		Type:     "php",
		OS:       "windows",
		Encoding: "gbk",
	}
	got := BuildWebshellAssistantContext(conn, WebshellSkillHintDefault, "list current directory and tell me where the flag is")

	mustContain(t, got,
		"[WebShell Assistant Context]",
		"ws_win01",
		"IIS Windows target",
		"Target system: Windows",
		"dir /a",
		"move /y",
		"avoid Unix commands like ls / cat / rm",
		"Response encoding: GBK",
		"backend auto-transcoded to UTF-8",
		`connection_id to "ws_win01"`,
		"webshell_exec",
		WebshellSkillHintDefault,
		"User request: list current directory and tell me where the flag is",
	)
	// Windows context should not mention Linux command recommendations
	mustNotContain(t, got, "recommended sh/bash")
}

func TestBuildWebshellAssistantContext_LinuxAutoFromPHP(t *testing.T) {
	conn := &database.WebShellConnection{
		ID:       "ws_lnx01",
		Remark:   "", // when Remark is empty, falls back to URL
		URL:      "http://example.com/a.php",
		Type:     "php",
		OS:       "auto", // auto + php → linux
		Encoding: "",     // auto encoding: no explicit hint
	}
	got := BuildWebshellAssistantContext(conn, WebshellSkillHintDefault, "view /etc/passwd")

	mustContain(t, got,
		"Connection ID: ws_lnx01",
		"Remark: http://example.com/a.php", // fallback to URL when Remark is empty
		"Target system: Linux/Unix",
		"ls -la",
		"mkdir -p",
		"avoid Windows commands like dir, type, del, move",
		"User request: view /etc/passwd",
	)
	// encoding=auto should not show "Response encoding:" line
	mustNotContain(t, got, "Response encoding:")
	// Linux context should not mention Windows commands
	mustNotContain(t, got, "recommended cmd/PowerShell")
}

func TestBuildWebshellAssistantContext_AutoFromASPDefaultsToWindows(t *testing.T) {
	// backward compatibility: old connections without os set, shellType=asp should be treated as Windows
	conn := &database.WebShellConnection{
		ID:       "ws_asp01",
		Remark:   "old ASP target",
		Type:     "asp",
		OS:       "", // empty string is equivalent to auto
		Encoding: "gb18030",
	}
	got := BuildWebshellAssistantContext(conn, WebshellSkillHintMultiAgent, "check current user")

	mustContain(t, got,
		"Target system: Windows",
		"Response encoding: GB18030",
		"backend auto-transcoded to UTF-8",
		WebshellSkillHintMultiAgent,
	)
	// multi-agent skill hint should not contain DeepAgent text from default hint
	mustNotContain(t, got, "DeepAgent")
}

func TestBuildWebshellAssistantContext_MultiAgentSkillHint(t *testing.T) {
	conn := &database.WebShellConnection{ID: "ws_m1", Remark: "x", Type: "php", OS: "linux"}
	got := BuildWebshellAssistantContext(conn, WebshellSkillHintMultiAgent, "hi")
	mustContain(t, got, WebshellSkillHintMultiAgent)
	mustNotContain(t, got, "DeepAgent")
}

func TestBuildWebshellAssistantContext_DefaultSkillHintFallback(t *testing.T) {
	conn := &database.WebShellConnection{ID: "ws_d1", Remark: "x", Type: "php", OS: "linux"}
	// passing empty skillHint should fall back to default
	got := BuildWebshellAssistantContext(conn, "", "hi")
	mustContain(t, got, WebshellSkillHintDefault)
}

func TestBuildWebshellAssistantContext_UTF8EncodingIsAnnotated(t *testing.T) {
	conn := &database.WebShellConnection{
		ID: "ws_u1", Remark: "u", Type: "jsp", OS: "linux", Encoding: "utf-8",
	}
	got := BuildWebshellAssistantContext(conn, WebshellSkillHintDefault, "hi")
	mustContain(t, got, "Response encoding: UTF-8", "target natively uses UTF-8")
}

func TestBuildWebshellAssistantContext_NilConnReturnsUserMsg(t *testing.T) {
	// defensive: conn == nil should not panic, should return original message
	got := BuildWebshellAssistantContext(nil, WebshellSkillHintDefault, "just the message")
	if got != "just the message" {
		t.Errorf("nil conn should return userMsg as-is, got %q", got)
	}
}

func TestDescribeTargetOSForPrompt(t *testing.T) {
	cases := map[string][]string{
		"windows": {"Windows", "dir /a", "move /y", "PowerShell"},
		"linux":   {"Linux/Unix", "ls -la", "mkdir -p"},
		"":        {"unknown", "uname"}, // defensive branch
	}
	for in, wants := range cases {
		got := describeTargetOSForPrompt(in)
		for _, w := range wants {
			if !strings.Contains(got, w) {
				t.Errorf("describeTargetOSForPrompt(%q) should contain %q, got: %s", in, w, got)
			}
		}
	}
}

func TestDescribeEncodingForPrompt(t *testing.T) {
	cases := map[string]string{
		"utf-8":   "UTF-8",
		"gbk":     "GBK",
		"gb18030": "GB18030",
		"auto":    "",
		"":        "",
	}
	for in, want := range cases {
		got := describeEncodingForPrompt(in)
		if want == "" && got != "" {
			t.Errorf("describeEncodingForPrompt(%q) should return empty string, got: %s", in, got)
		}
		if want != "" && !strings.Contains(got, want) {
			t.Errorf("describeEncodingForPrompt(%q) should contain %q, got: %s", in, want, got)
		}
	}
}

// ---- helpers ----

func mustContain(t *testing.T, text string, substrings ...string) {
	t.Helper()
	for _, s := range substrings {
		if !strings.Contains(text, s) {
			t.Errorf("expected text to contain %q\n--- text ---\n%s", s, text)
		}
	}
}

func mustNotContain(t *testing.T, text string, substrings ...string) {
	t.Helper()
	for _, s := range substrings {
		if strings.Contains(text, s) {
			t.Errorf("text should not contain %q\n--- text ---\n%s", s, text)
		}
	}
}

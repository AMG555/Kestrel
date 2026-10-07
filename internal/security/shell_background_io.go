package security

import "strings"

const backgroundJobStdioRedirect = " </dev/null >/dev/null 2>&1"

// findStandaloneAmpersandPositions returns the indices of standalone & characters not inside quotes (excluding &&).
func findStandaloneAmpersandPositions(command string) []int {
	command = strings.TrimSpace(command)
	if command == "" {
		return nil
	}

	var positions []int
	inSingleQuote := false
	inDoubleQuote := false
	escaped := false

	for i := 0; i < len(command); i++ {
		r := command[i]
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if r == '\'' && !inDoubleQuote {
			inSingleQuote = !inSingleQuote
			continue
		}
		if r == '"' && !inSingleQuote {
			inDoubleQuote = !inDoubleQuote
			continue
		}
		if r != '&' || inSingleQuote || inDoubleQuote {
			continue
		}
		if i+1 < len(command) && command[i+1] == '&' {
			continue
		}
		if i > 0 && command[i-1] == '&' {
			continue
		}

		isStandalone := i == 0
		if !isStandalone {
			prev := command[i-1]
			isStandalone = prev == ' ' || prev == '\t' || prev == '\n' || prev == '\r'
		}
		if !isStandalone {
			continue
		}
		if i == len(command)-1 {
			positions = append(positions, i)
			continue
		}
		next := command[i+1]
		if next == ' ' || next == '\t' || next == '\n' || next == '\r' {
			positions = append(positions, i)
		}
	}
	return positions
}

func segmentHasStdioRedirect(segment string) bool {
	lower := strings.ToLower(strings.TrimSpace(segment))
	if lower == "" {
		return false
	}
	if strings.Contains(lower, ">/dev/null") || strings.Contains(lower, "2>/dev/null") {
		return true
	}
	if strings.Contains(lower, "&>") || strings.Contains(lower, "&>>") {
		return true
	}
	if strings.Contains(lower, "2>&1") && strings.Contains(lower, "/dev/null") {
		return true
	}
	return false
}

// RedirectBackgroundJobStdio injects </dev/null >/dev/null 2>&1 before each standalone & background segment
// to prevent background child processes from holding the execute/exec pipe open and causing a deadlock.
func RedirectBackgroundJobStdio(command string) string {
	positions := findStandaloneAmpersandPositions(command)
	if len(positions) == 0 {
		return command
	}

	out := command
	for j := len(positions) - 1; j >= 0; j-- {
		i := positions[j]
		before := out[:i]
		after := out[i:]
		trimmed := strings.TrimRight(before, " \t\r\n")
		if segmentHasStdioRedirect(trimmed) {
			continue
		}
		trailing := before[len(trimmed):]
		out = trimmed + backgroundJobStdioRedirect + trailing + after
	}
	return out
}

// PrepareShellCommandForExecute combines non-interactive wrapping and background IO redirection for execute/exec.
// exec </dev/null must be injected first, then & background segments are rewritten; otherwise the </dev/null
// inside a segment would cause the stdin redirect to be mistakenly treated as already present.
func PrepareShellCommandForExecute(shellCommand string) string {
	return RedirectBackgroundJobStdio(PrepareNonInteractiveShellCommand(shellCommand))
}

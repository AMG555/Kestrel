package security

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// FormatCommandFailureResult produces failure text consistent with the exec tool ToolResult (without ToolErrorPrefix).
func FormatCommandFailureResult(exitCode int, output string) string {
	output = strings.TrimSpace(output)
	errMsg := fmt.Sprintf("exit status %d", exitCode)
	if output == "" {
		return fmt.Sprintf("command execution failed: %s", errMsg)
	}
	if strings.HasPrefix(output, "command execution failed:") {
		return output
	}
	return fmt.Sprintf("command execution failed: %s\nOutput: %s", errMsg, output)
}

// FormatCommandFailureFromErr generates a unified failure message from errors returned by exec/execute (IsError body).
func FormatCommandFailureFromErr(err error, output string) string {
	if err == nil {
		return strings.TrimSpace(output)
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return FormatCommandFailureResult(exitError.ExitCode(), output)
	}
	output = strings.TrimSpace(output)
	if output == "" {
		return fmt.Sprintf("command execution failed: %v", err)
	}
	if strings.HasPrefix(output, "command execution failed:") {
		return output
	}
	return fmt.Sprintf("command execution failed: %v\nOutput: %s", err, output)
}

// ExecuteFailureStatusLine is the single-line status appended at the end of a streaming execute (the output body has already been streamed).
func ExecuteFailureStatusLine(exitCode int) string {
	return fmt.Sprintf("\ncommand execution failed: exit status %d", exitCode)
}

// IsCommandFailureResult reports whether the tool result body indicates a non-zero command exit (used to align execute / exec isError).
func IsCommandFailureResult(content string) bool {
	return strings.Contains(content, "command execution failed:")
}

// IsLegacyShellExitNoise filters redundant exit-code lines from legacy shell streams.
func IsLegacyShellExitNoise(s string) bool {
	trimmed := strings.TrimSpace(s)
	return strings.HasPrefix(trimmed, "command exited with non-zero code ")
}

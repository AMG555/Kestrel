package security

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// FormatCommandFailureResult 与 exec tool ToolResult 文案一致（不含 ToolErrorPrefix）。
func FormatCommandFailureResult(exitCode int, output string) string {
	output = strings.TrimSpace(output)
	errMsg := fmt.Sprintf("exit status %d", exitCode)
	if output == "" {
		return fmt.Sprintf("command execution failed: %s", errMsg)
	}
	if strings.HasPrefix(output, "command execution failed:") {
		return output
	}
	return fmt.Sprintf("command execution failed: %s\n输出: %s", errMsg, output)
}

// FormatCommandFailureFromErr 根据 exec/execute back的 error 生成统一failed文案（IsError 正文）。
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
	return fmt.Sprintf("command execution failed: %v\n输出: %s", err, output)
}

// ExecuteFailureStatusLine 流式 execute 结束时追加的单行status（输出正文已在流中推送过）。
func ExecuteFailureStatusLine(exitCode int) string {
	return fmt.Sprintf("\ncommand execution failed: exit status %d", exitCode)
}

// IsCommandFailureResult 判断tool result正文yesno表示命令非零exit（用于 execute / exec 对齐 isError）。
func IsCommandFailureResult(content string) bool {
	return strings.Contains(content, "command execution failed:")
}

// IsLegacyShellExitNoise filter旧版 shell 流中冗余的 exit code 行。
func IsLegacyShellExitNoise(s string) bool {
	trimmed := strings.TrimSpace(s)
	return strings.HasPrefix(trimmed, "command exited with non-zero code ")
}

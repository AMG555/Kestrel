package multiagent

import "fmt"

// ExecuteExitError represents a non-zero exit from the execute command (expected failure, not timed out / interrupted / stream abnormal).
type ExecuteExitError struct {
	Code int
}

func (e *ExecuteExitError) Error() string {
	if e == nil {
		return "exit status unknown"
	}
	return fmt.Sprintf("exit status %d", e.Code)
}

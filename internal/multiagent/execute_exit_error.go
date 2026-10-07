package multiagent

import "fmt"

// ExecuteExitError 表示 execute 命令非零exit（预期failed，非timed out/中断/流abnormal）。
type ExecuteExitError struct {
	Code int
}

func (e *ExecuteExitError) Error() string {
	if e == nil {
		return "exit status unknown"
	}
	return fmt.Sprintf("exit status %d", e.Code)
}

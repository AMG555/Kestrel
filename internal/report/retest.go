package report

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"kestrel/internal/database"
)

// RetestResult represents the execution outcome of automated vulnerability verification.
type RetestResult struct {
	VulnerabilityID string `json:"vulnerability_id"`
	Target          string `json:"target"`
	IsResolved      bool   `json:"is_resolved"`
	Status          string `json:"status"` // "fixed" or "open"
	ExitCode        int    `json:"exit_code"`
	Output          string `json:"output"`
	Timestamp       string `json:"timestamp"`
	Message         string `json:"message"`
}

// RunVulnerabilityRetest executes the PoC reproduction script against the target to verify if the vulnerability has been resolved.
func RunVulnerabilityRetest(ctx context.Context, v *database.Vulnerability, targetOverride string) (*RetestResult, error) {
	if v == nil {
		return nil, fmt.Errorf("vulnerability is nil")
	}

	pyScript, err := GeneratePoC(v, "python")
	if err != nil {
		return nil, fmt.Errorf("failed to generate verification script: %w", err)
	}

	cmdArgs := []string{"-c", pyScript}
	if strings.TrimSpace(targetOverride) != "" {
		cmdArgs = append(cmdArgs, "--target", strings.TrimSpace(targetOverride))
	}

	pyExec := "python3"
	if _, err := exec.LookPath("python3"); err != nil {
		if _, err2 := exec.LookPath("python"); err2 == nil {
			pyExec = "python"
		}
	}

	cmd := exec.CommandContext(ctx, pyExec, cmdArgs...)
	out, execErr := cmd.CombinedOutput()
	outputStr := string(out)

	exitCode := 0
	if execErr != nil {
		if exitErr, ok := execErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	nowStr := time.Now().UTC().Format(time.RFC3339)
	result := &RetestResult{
		VulnerabilityID: v.ID,
		Target:          v.Target,
		ExitCode:        exitCode,
		Output:          outputStr,
		Timestamp:       nowStr,
	}

	// Exit code conventions from GeneratePoC:
	// Exit code 0: exploit indicator observed (still vulnerable)
	// Exit code 1: expected indicator absent (resolved / patched)
	// Exit code 2: connection failure
	if exitCode == 1 {
		result.IsResolved = true
		result.Status = "fixed"
		result.Message = "Vulnerability verified as RESOLVED: expected indicator was not triggered by target."
	} else if exitCode == 0 {
		result.IsResolved = false
		result.Status = "open"
		result.Message = "Vulnerability STILL ACTIVE: target returned exploit reproduction indicator."
	} else {
		result.IsResolved = false
		result.Status = v.Status
		result.Message = fmt.Sprintf("Verification inconclusive (target connection failure or error code %d).", exitCode)
	}

	return result, nil
}

package security

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// ShellNoOutputTimeoutMessage returns the message shown when no new stdout/stderr arrives for a prolonged period (soft failure, visible to the model).
func ShellNoOutputTimeoutMessage(idleSec int) string {
	return fmt.Sprintf(`Command terminated: no new output for %d seconds (possible interactive wait or hung process).

For long-running silent tasks, append & to run in the background, or increase agent.shell_no_output_timeout_seconds (-1 to disable this check).`, idleSec)
}

// ShellInactivityWatch signals expired when no new output arrives within noOutputSec; Bump resets the timer each time.
// Unlike "cancel the timer once the first chunk arrives", this also catches cases such as sudo printing a Password prompt then hanging.
type ShellInactivityWatch struct {
	Sec     int
	mu      sync.Mutex
	timer   *time.Timer
	Expired chan struct{}
}

func NewShellInactivityWatch(noOutputSec int) *ShellInactivityWatch {
	sec := ResolveShellNoOutputTimeoutSeconds(noOutputSec)
	if sec <= 0 {
		return nil
	}
	w := &ShellInactivityWatch{
		Sec:     sec,
		Expired: make(chan struct{}, 1),
	}
	w.Bump()
	return w
}

func (w *ShellInactivityWatch) Bump() {
	if w == nil || w.Sec <= 0 {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.timer != nil {
		w.timer.Stop()
	}
	w.timer = time.AfterFunc(time.Duration(w.Sec)*time.Second, func() {
		select {
		case w.Expired <- struct{}{}:
		default:
		}
	})
}

func (w *ShellInactivityWatch) Stop() {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.timer != nil {
		w.timer.Stop()
		w.timer = nil
	}
}

// ResolveShellNoOutputTimeoutSeconds: 0 = default 300 (5 minutes); -1 = disabled; >0 = custom.
func ResolveShellNoOutputTimeoutSeconds(sec int) int {
	if sec < 0 {
		return 0
	}
	if sec == 0 {
		return 300
	}
	return sec
}

// PrependNonInteractiveShellExports injects common non-interactive environment variables (pager etc.) for sh -c; does not maintain a command blacklist.
func PrependNonInteractiveShellExports(shellCommand string) string {
	if strings.TrimSpace(shellCommand) == "" {
		return shellCommand
	}
	upper := strings.ToUpper(shellCommand)
	var pairs []string
	add := func(key, val string) {
		if strings.Contains(upper, strings.ToUpper(key)) {
			return
		}
		pairs = append(pairs, key+"="+val)
	}
	add("GIT_PAGER", "cat")
	add("PAGER", "cat")
	add("SYSTEMD_PAGER", "cat")
	add("DEBIAN_FRONTEND", "noninteractive")
	if len(pairs) == 0 {
		return shellCommand
	}
	return "export " + strings.Join(pairs, " ") + "\n" + shellCommand
}

// PrependNonInteractiveStdinRedirect closes stdin for sh -c (equivalent to attachNonInteractiveStdin),
// so programs that read from stdin such as read/input()/sudo -S fail fast rather than hanging. No-op if </dev/null is already present.
func PrependNonInteractiveStdinRedirect(shellCommand string) string {
	if strings.TrimSpace(shellCommand) == "" {
		return shellCommand
	}
	lower := strings.ToLower(shellCommand)
	if strings.Contains(lower, "</dev/null") || strings.Contains(lower, "0</dev/null") {
		return shellCommand
	}
	return "exec </dev/null\n" + shellCommand
}

// PrepareNonInteractiveShellCommand combines non-interactive wrapping: close stdin + pager env vars (no command blacklist).
func PrepareNonInteractiveShellCommand(shellCommand string) string {
	return PrependNonInteractiveStdinRedirect(PrependNonInteractiveShellExports(shellCommand))
}

// ApplyNonInteractivePagerEnv adds the same env vars to exec.Cmd as PrependNonInteractiveShellExports.
func ApplyNonInteractivePagerEnv(cmdEnv []string) []string {
	if cmdEnv == nil {
		cmdEnv = []string{}
	}
	has := func(k string) bool {
		prefix := k + "="
		for _, e := range cmdEnv {
			if strings.HasPrefix(e, prefix) {
				return true
			}
		}
		return false
	}
	if !has("GIT_PAGER") {
		cmdEnv = append(cmdEnv, "GIT_PAGER=cat")
	}
	if !has("PAGER") {
		cmdEnv = append(cmdEnv, "PAGER=cat")
	}
	if !has("SYSTEMD_PAGER") {
		cmdEnv = append(cmdEnv, "SYSTEMD_PAGER=cat")
	}
	if !has("DEBIAN_FRONTEND") {
		cmdEnv = append(cmdEnv, "DEBIAN_FRONTEND=noninteractive")
	}
	return cmdEnv
}

// attachNonInteractiveStdin closes interactive stdin so commands that would wait for input fail fast instead.
func attachNonInteractiveStdin(cmd *exec.Cmd) {
	if cmd == nil || cmd.Stdin != nil {
		return
	}
	f, err := os.Open(os.DevNull)
	if err != nil {
		return
	}
	cmd.Stdin = f
}

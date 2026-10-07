package multiagent

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"regexp"
	"strconv"
	"strings"
	"time"

	"kestrel/internal/config"

	einoopenai "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

const (
	defaultEinoRunRetryMaxAttempts = 4
	defaultEinoRunRetryMaxBackoff  = 30 * time.Second
)

var httpStatusInErrorPattern = regexp.MustCompile(`(?i)(?:http|status(?:\s+code)?|upstream\s+returned)\s*[:=]?\s*(\d{3})\b`)

// isEinoTransientRunError is the sole criterion for Eino runtime 'backoff retry vs fail immediately'.
// Returns true for 429/5xx/network jitter etc.; returns false for user-cancelled, timed out, iteration limit, auth failure etc.
// Other modules (run loop, summarization etc.) only call this function; no parallel rules are maintained elsewhere.
func isEinoTransientRunError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if errors.Is(err, adk.ErrExceedMaxRetries) {
		return false
	}
	if _, ok := isEinoNativeWillRetry(err); ok {
		return false
	}
	if isEinoIterationLimitError(err) {
		return false
	}
	err = unwrapEinoRetryExhausted(err)
	var apiErr *einoopenai.APIError
	if errors.As(err, &apiErr) && apiErr.HTTPStatusCode > 0 {
		return isRetryableHTTPStatus(apiErr.HTTPStatusCode)
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	if msg == "" {
		return false
	}
	if isEinoEmptySummaryContentErrorText(msg) {
		return true
	}
	if status := httpStatusFromErrorText(msg); status > 0 {
		return isRetryableHTTPStatus(status)
	}
	transientMarkers := []string{
		"too many requests",
		"rate limit",
		"rate_limit",
		"ratelimit",
		"overloaded",
		"capacity",
		"temporarily unavailable",
		"service unavailable",
		"bad gateway",
		"gateway timeout",
		"internal server error",
		"unexpected internal error",
		"connection reset",
		"connection refused",
		"connection closed",
		"i/o timeout",
		"no such host",
		"network is unreachable",
		"broken pipe",
		"read tcp",
		"write tcp",
		"dial tcp",
		"tls handshake timeout",
		"stream error",
		"failed to receive stream chunk",
		"goaway", // http2: server sent GOAWAY and closed the connection
		"unexpected eof",
		`": eof`, // net/http: Post "url": EOF (often wraps io.EOF)
		"unexpected end of json",
	}
	for _, m := range transientMarkers {
		if strings.Contains(msg, m) {
			return true
		}
	}
	return false
}

func isEinoEmptySummaryContentErrorText(msg string) bool {
	return strings.Contains(msg, "summary content is empty") ||
		strings.Contains(msg, "agentic summarization returned empty summary")
}

func isRetryableHTTPStatus(status int) bool {
	switch status {
	case 408, 409, 425, 429:
		return true
	default:
		return status >= 500 && status <= 599
	}
}

func einoTransientRunErrorUserDetail(err error) (kind, summary string) {
	if err == nil {
		return "", ""
	}
	msg := strings.TrimSpace(err.Error())
	lower := strings.ToLower(msg)
	if status := httpStatusFromErrorText(lower); status > 0 {
		switch {
		case status == 429:
			kind = "rate_limit"
		case status == 408 || status == 409 || status == 425:
			kind = "retryable_http"
		case status >= 500 && status <= 599:
			kind = "upstream_server"
		default:
			kind = "http_error"
		}
	} else {
		var apiErr *einoopenai.APIError
		if errors.As(err, &apiErr) && apiErr.HTTPStatusCode > 0 {
			switch {
			case apiErr.HTTPStatusCode == 429:
				kind = "rate_limit"
			case apiErr.HTTPStatusCode == 408 || apiErr.HTTPStatusCode == 409 || apiErr.HTTPStatusCode == 425:
				kind = "retryable_http"
			case apiErr.HTTPStatusCode >= 500 && apiErr.HTTPStatusCode <= 599:
				kind = "upstream_server"
			default:
				kind = "http_error"
			}
		}
	}
	if kind == "" {
		switch {
		case strings.Contains(lower, "too many requests") ||
			strings.Contains(lower, "rate limit") ||
			strings.Contains(lower, "rate_limit") ||
			strings.Contains(lower, "ratelimit"):
			kind = "rate_limit"
		case strings.Contains(lower, "overloaded") ||
			strings.Contains(lower, "capacity") ||
			strings.Contains(lower, "temporarily unavailable") ||
			strings.Contains(lower, "service unavailable") ||
			strings.Contains(lower, "unexpected internal error"):
			kind = "upstream_busy"
		case strings.Contains(lower, "connection reset") ||
			strings.Contains(lower, "connection refused") ||
			strings.Contains(lower, "connection closed") ||
			strings.Contains(lower, "i/o timeout") ||
			strings.Contains(lower, "no such host") ||
			strings.Contains(lower, "network is unreachable") ||
			strings.Contains(lower, "broken pipe") ||
			strings.Contains(lower, "read tcp") ||
			strings.Contains(lower, "write tcp") ||
			strings.Contains(lower, "dial tcp") ||
			strings.Contains(lower, "tls handshake timeout") ||
			strings.Contains(lower, "goaway") ||
			strings.Contains(lower, "unexpected eof"):
			kind = "network"
		case strings.Contains(lower, "stream error") ||
			strings.Contains(lower, "failed to receive stream chunk") ||
			strings.Contains(lower, "unexpected end of json"):
			kind = "stream"
		default:
			kind = "transient"
		}
	}
	return kind, einoTrimRetryErrorSummary(msg)
}

func einoTrimRetryErrorSummary(msg string) string {
	msg = strings.Join(strings.Fields(strings.TrimSpace(msg)), " ")
	const maxRunes = 500
	runes := []rune(msg)
	if len(runes) <= maxRunes {
		return msg
	}
	return string(runes[:maxRunes]) + "..."
}

func httpStatusFromErrorText(msg string) int {
	match := httpStatusInErrorPattern.FindStringSubmatch(msg)
	if len(match) != 2 {
		return 0
	}
	status, _ := strconv.Atoi(match[1])
	return status
}

type einoTransientRunRetryPolicy struct {
	maxAttempts int
	maxBackoff  time.Duration
}

func einoTransientRunRetryPolicyFromArgs(args *einoADKRunLoopArgs) einoTransientRunRetryPolicy {
	return einoTransientRunRetryPolicy{
		maxAttempts: einoRunRetryMaxAttempts(args),
		maxBackoff:  einoRunRetryMaxBackoff(args),
	}
}

func einoTransientRunRetryPolicyFromMW(mw *config.MultiAgentEinoMiddlewareConfig) einoTransientRunRetryPolicy {
	return einoTransientRunRetryPolicy{
		maxAttempts: RunRetryMaxAttemptsFromConfig(mw),
		maxBackoff:  einoRunRetryMaxBackoffFromConfig(mw),
	}
}

// einoTransientRunRetrier applies exponential backoff to transient errors within the run loop and restarts the Runner (the only retry execution layer).
type einoTransientRunRetrier struct {
	policy   einoTransientRunRetryPolicy
	attempts int
}

func newEinoTransientRunRetrier(policy einoTransientRunRetryPolicy) *einoTransientRunRetrier {
	return &einoTransientRunRetrier{policy: policy}
}

// tryRetry backs off a transient error and returns a restart message; returns an exhausted error when attempts are used up.
func (r *einoTransientRunRetrier) tryRetry(
	ctx context.Context,
	runErr error,
	args *einoADKRunLoopArgs,
	baseMsgs, accumulated []adk.Message,
	baseCount int,
) (restarted bool, restartMsgs []adk.Message, ctxSource einoRunRestartContextSource, backoff time.Duration, fatal error) {
	if runErr == nil || !isEinoTransientRunError(runErr) {
		return false, nil, "", 0, runErr
	}
	r.attempts++
	if r.attempts > r.policy.maxAttempts {
		return false, nil, "", 0, fmt.Errorf("transient retry exhausted after %d attempts: %w", r.policy.maxAttempts, runErr)
	}
	backoff = einoTransientRetryBackoff(r.attempts-1, r.policy.maxBackoff)
	select {
	case <-ctx.Done():
		return false, nil, "", 0, ctx.Err()
	case <-time.After(backoff):
	}
	restartMsgs, ctxSource = einoMessagesForRunRestart(args, baseMsgs, accumulated, baseCount)
	return true, restartMsgs, ctxSource, backoff, nil
}

func (r *einoTransientRunRetrier) attempt() int { return r.attempts }

func (r *einoTransientRunRetrier) maxAttempts() int { return r.policy.maxAttempts }

// reset clears the counter after a successful advance post-backoff (stream/message fully received), so subsequent transient errors restart from the 1st backoff.
func (r *einoTransientRunRetrier) reset() { r.attempts = 0 }

func einoRunRetryMaxAttempts(args *einoADKRunLoopArgs) int {
	if args != nil && args.RunRetryMaxAttempts > 0 {
		return args.RunRetryMaxAttempts
	}
	return defaultEinoRunRetryMaxAttempts
}

// RunRetryMaxAttemptsFromConfig returns the native model retry count, with legacy run_retry_max_attempts as a fallback.
func RunRetryMaxAttemptsFromConfig(mw *config.MultiAgentEinoMiddlewareConfig) int {
	if mw != nil {
		if mw.ModelRetryMaxRetries > 0 {
			return mw.ModelRetryMaxRetries
		}
		if mw.RunRetryMaxAttempts > 0 {
			return mw.RunRetryMaxAttempts
		}
	}
	return defaultEinoRunRetryMaxAttempts
}

func einoRunRetryMaxBackoff(args *einoADKRunLoopArgs) time.Duration {
	if args != nil && args.RunRetryMaxBackoffSec > 0 {
		return time.Duration(args.RunRetryMaxBackoffSec) * time.Second
	}
	return defaultEinoRunRetryMaxBackoff
}

func einoRunRetryMaxBackoffFromConfig(mw *config.MultiAgentEinoMiddlewareConfig) time.Duration {
	if mw != nil {
		if mw.ModelRetryMaxBackoffSec > 0 {
			return time.Duration(mw.ModelRetryMaxBackoffSec) * time.Second
		}
		if mw.RunRetryMaxBackoffSec > 0 {
			return time.Duration(mw.RunRetryMaxBackoffSec) * time.Second
		}
	}
	return defaultEinoRunRetryMaxBackoff
}

// einoRunRestartContextSource describes the message source used by Run on restart without a checkpoint Resume (log/SSE).
type einoRunRestartContextSource string

const (
	einoRestartContextInitial     einoRunRestartContextSource = "initial"
	einoRestartContextAccumulated einoRunRestartContextSource = "accumulated"
	einoRestartContextModelTrace  einoRunRestartContextSource = "model_trace"
)

// einoMessagesForRunRestart selects the most complete context when re-running after backoff:
// 1) ModelFacingTrace (consistent with model's actual input) 2) event-stream-accumulated runAccumulatedMsgs 3) initial msgs.
func einoMessagesForRunRestart(args *einoADKRunLoopArgs, baseMsgs, accumulated []adk.Message, baseCount int) ([]adk.Message, einoRunRestartContextSource) {
	if trace := modelFacingTraceSnapshot(args); len(trace) > 0 {
		// modelFacingTrace includes prior Instruction system message(s); genModelInput will prepend again.
		return stripADKSystemMessages(trace), einoRestartContextModelTrace
	}
	if len(accumulated) > baseCount {
		return stripADKSystemMessages(accumulated), einoRestartContextAccumulated
	}
	return append([]adk.Message(nil), baseMsgs...), einoRestartContextInitial
}

// adkMessagesHasUserContent reports whether the conversation tail is already a user turn
// with the given content. Only the last message counts: matching text in an earlier round
// (e.g. user repeats the same prompt after an assistant reply) must not suppress appending
// the new user turn — Claude 4.6+ rejects requests whose final message is assistant.
func adkMessagesHasUserContent(msgs []adk.Message, want string) bool {
	want = strings.TrimSpace(want)
	if want == "" {
		return true
	}
	if len(msgs) == 0 {
		return false
	}
	last := msgs[len(msgs)-1]
	if last == nil || last.Role != schema.User {
		return false
	}
	return strings.TrimSpace(last.Content) == want
}

// appendUserMessageIfNeeded appends this round's user message after the history trace (only when the tail already has the same user sentence).
func appendUserMessageIfNeeded(msgs []adk.Message, userMessage string) []adk.Message {
	if strings.TrimSpace(userMessage) == "" || adkMessagesHasUserContent(msgs, userMessage) {
		return msgs
	}
	return append(msgs, schema.UserMessage(userMessage))
}

// einoTransientRetryBackoff uses equal-jitter exponential backoff. Jitter avoids
// synchronized retries when many conversations hit the same provider limit.
func einoTransientRetryBackoff(attempt int, maxBackoff time.Duration) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	if attempt > 30 {
		attempt = 30
	}
	ceiling := time.Duration(1<<uint(attempt+1)) * time.Second
	if maxBackoff > 0 && ceiling > maxBackoff {
		ceiling = maxBackoff
	}
	if ceiling <= 1 {
		return ceiling
	}
	half := ceiling / 2
	return half + time.Duration(rand.Int64N(int64(ceiling-half)+1))
}

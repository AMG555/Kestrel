package openai

// eino_sse_sanitizer.go resolves an issue when Eino uses the meguminnnnnnnnn/go-openai SDK:
// proxy heartbeat/SSE control lines accumulating > 300 lines trigger ErrTooManyEmptyStreamMessages
// （报错文案: "stream has sent too many empty messages"）的问题。
//
// Trigger chain:
//   einoopenai.NewChatModel
//     → eino-ext/libs/acl/openai → meguminnnnnnnnn/go-openai
//     → streamReader.processLines() counts all non-"data:" lines; throws when count > 300.
//
// Common non-data: lines from proxies (valid SSE but rejected by the SDK):
//   ":" / ": keepalive" / ": ping" / "event: ping" / "retry: 3000"
//   and large numbers of heartbeats interspersed during thinking-model prefill.
//
// Mitigation: at the HTTP transport layer, wrap the response Body in a reader that only passes through "data:"
// lines and silently discards heartbeats/comments/event-type lines. The downstream SDK never sees non-data: lines,
// so the counter stays at 0 and this error can never occur again.
//
// This layer is completely transparent to callers:
//   - only intervenes when the response Content-Type is text/event-stream; ordinary JSON responses pass through unchanged
//   - data: payload (including [DONE] and {"error":...}) is passed through byte-for-byte
//   - upstream real stream breaks (EOF / connection reset / context cancel) are passed through as-is

import (
	"bufio"
	"bytes"
	"io"
	"net/http"
	"strings"
)

const (
	// einoSSEReaderBufSize gives bufio a larger initial buffer to avoid frequent reallocations for single large JSON chunk lines
	// (containing tool call arguments / reasoning_content).
	einoSSEReaderBufSize = 64 * 1024
)

// einoSSESanitizingRoundTripper wraps the downstream RoundTripper and performs line-level sanitisation on SSE responses.
type einoSSESanitizingRoundTripper struct {
	base http.RoundTripper
}

func (rt *einoSSESanitizingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := rt.base.RoundTrip(req)
	if err != nil || resp == nil {
		return resp, err
	}
	if !isSSEResponse(resp) {
		return resp, nil
	}
	resp.Body = newEinoSSESanitizingBody(resp.Body)
	return resp, nil
}

// isSSEResponse only sanitises responses with status 200 + text/event-stream;
// error responses are handled by a separate einoSSEErrorRoundTripper; this layer does not touch them.
func isSSEResponse(resp *http.Response) bool {
	if resp.StatusCode != http.StatusOK {
		return false
	}
	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		return false
	}
	ct = strings.ToLower(strings.TrimSpace(ct))
	// Handles "text/event-stream", "text/event-stream; charset=utf-8", etc.
	return strings.HasPrefix(ct, "text/event-stream")
}

// einoSSESanitizingBody is the wrapped response body: only passes through data: lines, discards all others.
type einoSSESanitizingBody struct {
	upstream io.ReadCloser
	reader   *bufio.Reader
	pending  []byte // sanitised bytes ready to return to downstream (always a complete data: line ending with \n)
	err      error  // upstream terminal error (io.EOF or network error)
}

func newEinoSSESanitizingBody(body io.ReadCloser) *einoSSESanitizingBody {
	return &einoSSESanitizingBody{
		upstream: body,
		reader:   bufio.NewReaderSize(body, einoSSEReaderBufSize),
	}
}

func (b *einoSSESanitizingBody) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if len(b.pending) > 0 {
		n := copy(p, b.pending)
		b.pending = b.pending[n:]
		return n, nil
	}

	// Read from upstream until a data: line is accumulated or a terminal state is reached.
	// A single loop iteration may discard any number of heartbeat lines but exits after passing through at most one data: line,
	// to avoid excessive blocking or a large pending buffer in a single Read call.
	for b.err == nil {
		line, err := b.reader.ReadBytes('\n')
		if len(line) > 0 {
			if isPassThroughSSELine(line) {
				if line[len(line)-1] != '\n' {
					line = append(line, '\n')
				}
				b.pending = line
				if err != nil {
					b.err = err
				}
				break
			}
			// Non-data: lines (empty lines / ":" comments / event: / retry: / id: / any bare text)
			// all discarded; not exposed to downstream; continue reading the next line.
		}
		if err != nil {
			b.err = err
			break
		}
	}

	if len(b.pending) > 0 {
		n := copy(p, b.pending)
		b.pending = b.pending[n:]
		return n, nil
	}
	return 0, b.err
}

func (b *einoSSESanitizingBody) Close() error {
	return b.upstream.Close()
}

// isPassThroughSSELine determines whether a line should be passed through to the downstream SDK unchanged.
// Only lines starting with "data:" (case-insensitive, any leading whitespace allowed) are kept.
// Note: TrimSpace must not remove trailing newlines before checking, otherwise "  data: x" would be misclassified;
// only trim leading whitespace, consistent with the SDK's internal TrimSpace + ^data:\s* regex semantics.
func isPassThroughSSELine(line []byte) bool {
	trimmed := bytes.TrimLeft(line, " \t")
	if len(trimmed) < 5 {
		return false
	}
	// Case-insensitive comparison of the first 5 bytes for "data:". SSE spec requires lowercase field names,
	// but loose matching handles non-standard proxy implementations.
	return (trimmed[0] == 'd' || trimmed[0] == 'D') &&
		(trimmed[1] == 'a' || trimmed[1] == 'A') &&
		(trimmed[2] == 't' || trimmed[2] == 'T') &&
		(trimmed[3] == 'a' || trimmed[3] == 'A') &&
		trimmed[4] == ':'
}

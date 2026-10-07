package openai

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

// Reproduces the meguminnnnnnnnn/go-openai SSE line-counting algorithm (default limit=300):
// - read line by line
// - non-"data:" lines (empty lines / ":" comments / event: / retry:) increment emptyMessagesCount
// - > 300 throws ErrTooManyEmptyStreamMessages
// - on "data:" line, reset counter and return payload
//
// This algorithm matches processLines() in the upstream SDK's stream_reader.go exactly (see
// /Users/temp/go/pkg/mod/github.com/meguminnnnnnnnn/go-openai@v0.1.2/stream_reader.go).
// The test only reproduces the "limit triggered" behavior to regression-test the sanitizer root-cause fix.
var errTooManyEmptyStreamMessages = errors.New("stream has sent too many empty messages")

func sdkLikeRecvAll(body io.Reader, limit uint) ([]string, error) {
	headerData := regexp.MustCompile(`^data:\s*`)
	r := bufio.NewReader(body)
	var payloads []string
	for {
		var emptyMessagesCount uint
		var payload []byte
		for {
			line, err := r.ReadBytes('\n')
			if err != nil {
				if err == io.EOF {
					return payloads, nil
				}
				return payloads, err
			}
			noSpace := bytes.TrimSpace(line)
			if !headerData.Match(noSpace) {
				emptyMessagesCount++
				if emptyMessagesCount > limit {
					return payloads, errTooManyEmptyStreamMessages
				}
				continue
			}
			payload = headerData.ReplaceAll(noSpace, nil)
			break
		}
		if string(payload) == "[DONE]" {
			return payloads, nil
		}
		payloads = append(payloads, string(payload))
	}
}

func newSSEServer(t *testing.T, body string, contentType string, status int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
}

func sanitizingClient(base *http.Client) *http.Client {
	if base == nil {
		base = &http.Client{}
	}
	cloned := *base
	transport := base.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	cloned.Transport = &einoSSESanitizingRoundTripper{base: transport}
	return &cloned
}

func readAll(t *testing.T, body io.ReadCloser) string {
	t.Helper()
	defer body.Close()
	out, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(out)
}

// 1) Only "data:" lines → pass through byte-for-byte unchanged.
func TestSSESanitizer_PassesDataLinesUnchanged(t *testing.T) {
	body := "data: {\"a\":1}\ndata: {\"b\":2}\ndata: [DONE]\n"
	srv := newSSEServer(t, body, "text/event-stream", 200)
	defer srv.Close()

	resp, err := sanitizingClient(nil).Get(srv.URL)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	got := readAll(t, resp.Body)
	if got != body {
		t.Fatalf("body mismatch:\nwant %q\ngot  %q", body, got)
	}
}

// 2) Heartbeat/comment/event-type lines are dropped; only "data:" lines are retained.
func TestSSESanitizer_DropsHeartbeatsAndControlLines(t *testing.T) {
	body := strings.Join([]string{
		": keepalive",
		"",
		"event: ping",
		"retry: 3000",
		"id: 42",
		"data: {\"x\":1}",
		": ping",
		"",
		"data: {\"x\":2}",
		"data: [DONE]",
		"",
	}, "\n")
	srv := newSSEServer(t, body, "text/event-stream", 200)
	defer srv.Close()

	resp, err := sanitizingClient(nil).Get(srv.URL)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	got := readAll(t, resp.Body)
	want := "data: {\"x\":1}\ndata: {\"x\":2}\ndata: [DONE]\n"
	if got != want {
		t.Fatalf("sanitized body mismatch:\nwant %q\ngot  %q", want, got)
	}
}

// 3) Root-cause regression: upstream queues 500 heartbeat lines before sending data:;
// the original SDK algorithm would throw ErrTooManyEmptyStreamMessages; after sanitization all data: must be received.
func TestSSESanitizer_ProtectsAgainstTooManyEmptyMessages(t *testing.T) {
	const heartbeats = 500
	var buf bytes.Buffer
	for i := 0; i < heartbeats; i++ {
		buf.WriteString(": keepalive\n")
	}
	buf.WriteString("data: {\"chunk\":1}\n")
	buf.WriteString("data: {\"chunk\":2}\n")
	buf.WriteString("data: [DONE]\n")

	t.Run("baseline_without_sanitizer_must_fail", func(t *testing.T) {
		_, err := sdkLikeRecvAll(bytes.NewReader(buf.Bytes()), 300)
		if !errors.Is(err, errTooManyEmptyStreamMessages) {
			t.Fatalf("expected ErrTooManyEmptyStreamMessages, got %v", err)
		}
	})

	t.Run("with_sanitizer_must_succeed", func(t *testing.T) {
		srv := newSSEServer(t, buf.String(), "text/event-stream", 200)
		defer srv.Close()

		resp, err := sanitizingClient(nil).Get(srv.URL)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		defer resp.Body.Close()

		payloads, err := sdkLikeRecvAll(resp.Body, 300)
		if err != nil {
			t.Fatalf("sdk-like recv after sanitize: %v", err)
		}
		want := []string{`{"chunk":1}`, `{"chunk":2}`}
		if len(payloads) != len(want) {
			t.Fatalf("payload count mismatch: want %d got %d (%v)", len(want), len(payloads), payloads)
		}
		for i, w := range want {
			if payloads[i] != w {
				t.Fatalf("payload[%d] mismatch: want %q got %q", i, w, payloads[i])
			}
		}
	})
}

// 4) Heartbeats interleaved with data: lines are also cleaned correctly (common during thinking-model prefill).
func TestSSESanitizer_HeartbeatsInterleavedWithData(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteString("data: {\"chunk\":1}\n")
	for i := 0; i < 400; i++ {
		buf.WriteString(": keepalive\n")
	}
	buf.WriteString("data: {\"chunk\":2}\n")
	buf.WriteString("data: [DONE]\n")

	srv := newSSEServer(t, buf.String(), "text/event-stream", 200)
	defer srv.Close()

	resp, err := sanitizingClient(nil).Get(srv.URL)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()

	payloads, err := sdkLikeRecvAll(resp.Body, 300)
	if err != nil {
		t.Fatalf("sdk-like recv: %v", err)
	}
	if got, want := len(payloads), 2; got != want {
		t.Fatalf("payload count: want %d got %d", want, got)
	}
}

// 5) Non-SSE responses (e.g. non-streaming JSON) must not be touched by the sanitizer.
func TestSSESanitizer_PassesNonSSEResponseUntouched(t *testing.T) {
	body := `{"id":"x","object":"chat.completion","choices":[]}`
	srv := newSSEServer(t, body, "application/json", 200)
	defer srv.Close()

	resp, err := sanitizingClient(nil).Get(srv.URL)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	got := readAll(t, resp.Body)
	if got != body {
		t.Fatalf("non-SSE body must be untouched:\nwant %q\ngot  %q", body, got)
	}
}

// 6) Error responses (4xx/5xx) must not be sanitized, even if Content-Type is SSE,
//    to avoid swallowing error bodies that contain text other than "data:".
func TestSSESanitizer_PassesNon200Untouched(t *testing.T) {
	body := `{"error":{"message":"rate limit"}}`
	srv := newSSEServer(t, body, "text/event-stream", 429)
	defer srv.Close()

	resp, err := sanitizingClient(nil).Get(srv.URL)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	got := readAll(t, resp.Body)
	if got != body {
		t.Fatalf("error body must be untouched:\nwant %q\ngot  %q", body, got)
	}
}

// 7) If a data: line is missing a trailing \n (abnormal upstream), the sanitizer appends one to ensure downstream line parsing works.
func TestSSESanitizer_AppendsTrailingNewlineIfMissing(t *testing.T) {
	body := "data: {\"a\":1}"
	srv := newSSEServer(t, body, "text/event-stream", 200)
	defer srv.Close()

	resp, err := sanitizingClient(nil).Get(srv.URL)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	got := readAll(t, resp.Body)
	want := "data: {\"a\":1}\n"
	if got != want {
		t.Fatalf("trailing newline:\nwant %q\ngot  %q", want, got)
	}
}

// 8) Large chunks (tens of KB per line) are passed through completely without truncation.
func TestSSESanitizer_LargeDataLinePassesIntact(t *testing.T) {
	huge := strings.Repeat("x", 80*1024)
	body := "data: {\"big\":\"" + huge + "\"}\ndata: [DONE]\n"
	srv := newSSEServer(t, body, "text/event-stream", 200)
	defer srv.Close()

	resp, err := sanitizingClient(nil).Get(srv.URL)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	got := readAll(t, resp.Body)
	if got != body {
		t.Fatalf("large body length mismatch: want %d got %d", len(body), len(got))
	}
}

// 9) Unit coverage for isPassThroughSSELine.
func TestIsPassThroughSSELine(t *testing.T) {
	cases := []struct {
		line string
		want bool
	}{
		{"data: {\"a\":1}\n", true},
		{"DATA: x\n", true},
		{"  data: x\n", true},
		{"data:\n", true},
		{"\n", false},
		{"\r\n", false},
		{": keepalive\n", false},
		{":\n", false},
		{"event: ping\n", false},
		{"retry: 3000\n", false},
		{"id: 42\n", false},
		{"datax: y\n", false},
		{"da", false},
	}
	for _, c := range cases {
		if got := isPassThroughSSELine([]byte(c.line)); got != c.want {
			t.Errorf("isPassThroughSSELine(%q) = %v, want %v", c.line, got, c.want)
		}
	}
}

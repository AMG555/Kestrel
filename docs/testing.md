# Testing Guide

Testing Kestrel means more than running Go tests. Agent, MCP, HITL, C2, WebShell, and frontend streaming all have different failure modes.

## Prerequisites

### CGO / SQLite (required for database tests)

The database layer uses `go-sqlite3`, which requires a C compiler. Tests that touch SQLite will fail with:

```
Binary was compiled with 'CGO_ENABLED=0', go-sqlite3 requires cgo to work. This is a stub
```

**Linux / macOS**: `gcc` or `clang` is usually pre-installed — tests run normally.

**Windows**: Install [TDM-GCC](https://jmeubank.github.io/tdm-gcc/) or [MSYS2](https://www.msys2.org/) and ensure `gcc` is on `%PATH%`, then run:

```powershell
$env:CGO_ENABLED = "1"
go test ./...
```

CI runs with `CGO_ENABLED=1` on Linux and the full suite passes green.

### Shell tests (`internal/security`)

Tests in `internal/security` spawn `/bin/sh` or `sh` — they are Linux/macOS-only by design and are **expected to fail on Windows**. This is not a bug; these tests cover the Unix shell execution boundary that the production deployment uses.

---

## Commands

```bash
go test ./internal/...
go test ./cmd/...
go build -o kestrel ./cmd/server
```

Run focused packages when working locally:

```bash
go test ./internal/multiagent
go test ./internal/handler
go test ./internal/security
```

## Test Pyramid

| Layer | Goal | Example |
| --- | --- | --- |
| Unit | pure logic | expressions, chunking, sanitization |
| Handler | HTTP behavior | validation, auth, status codes |
| Integration | module cooperation | external MCP, KB indexing, HITL |
| Smoke | user path | login, chat, tools, settings |
| Authorized lab | high-risk features | C2, WebShell, terminal |

Do not use end-to-end manual testing as a substitute for unit tests, or unit tests as a substitute for high-risk lab validation.

## Regression Focus

Expand testing when changing:

- `internal/handler/config.go`: model, KB, MCP, C2, robot apply paths;
- `internal/multiagent/`: streaming, tool calls, summarization, retry, HITL;
- `internal/security/`: auth, shell, timeout, no-output;
- `internal/database/`: old data compatibility;
- `web/static/js/chat.js`: chat, process details, attack chain, groups.

## Test Data

Avoid real customer data. Prepare:

- small Markdown KB sample;
- fake local MCP server;
- controlled local HTTP target;
- harmless WebShell simulator;
- temporary SQLite DB.

## Failure Cases

Cover:

- model API 401/429/500;
- MCP startup failure;
- tool timeout;
- HITL rejection;
- interrupted KB indexing;
- unwritable database;
- WebShell non-200 response;
- C2 disabled endpoint access.

## Source Anchors

Existing tests live across:

- `internal/handler/*_test.go`
- `internal/multiagent/*_test.go`
- `internal/workflow/*_test.go`
- `internal/knowledge/*_test.go`
- `internal/security/*_test.go`
- `internal/mcp/*_test.go`
- `internal/c2/*_test.go`

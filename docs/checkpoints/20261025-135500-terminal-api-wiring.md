---
status: completed
branch: main
timestamp: 2026-10-25T13:55:00+05:30
commits:
  - b3f8f65: fix(terminal): wire execTerminal API endpoint, support custom timeout, and track duration_ms
---

## Working on: terminal-api-wiring

### Summary
Milestone 11 completed. Resolved critical gap identified during deep audit in Operator Terminal: `api.execTerminal` was missing from `web/src/api.js`, causing frontend commands to fail with `api.execTerminal is not a function`. Additionally enhanced backend terminal handler to respect request timeouts and compute execution duration in milliseconds.

### Completed Work
- [x] Added `execTerminal: (d) => request('POST', '/terminal/run', d)` in `web/src/api.js`.
- [x] Enhanced `internal/handler/terminal.go`:
  - Added `Timeout` field to `RunCommandRequest` allowing per-command timeout isolation.
  - Added `DurationMs` field to `RunCommandResponse`.
  - Added execution duration tracking (`durationMs := time.Since(startTime).Milliseconds()`).
  - Corrected timeout error message to reflect actual request timeout limit.
- [x] Updated `web/src/pages/Terminal.jsx`:
  - Updated API import path with explicit `.js` extension.
  - Validated clean Vite production bundle (`npm run build`).

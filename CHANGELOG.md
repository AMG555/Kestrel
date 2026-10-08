# Changelog

All notable changes to Kestrel are documented here.
Format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

---

## [Unreleased] — under development

### Fixed
- `TestBuildToolFailureMessageAuthorizationDenied` — test assertion corrected from `"errordetails:"` to `"error details:"` to match the actual output format of `buildToolFailureMessage`
- `TestEnrichSpecWithI18nKeysForAssetImport` — added `"Asset count or field validation failed"` alias to the i18n response-description map so the 400 response is correctly keyed to `assetImportValidationFailed`
- `TestToolGuardSavePersistsAndAppliesWithoutChangingHITL` — permission check now skips the Unix mode comparison on Windows, where `Chmod` is a no-op
- `TestEinoTransientRunRetryHandlerPreparesRetry` — corrected argument order in `emitEinoRunRetryProgress` format string; message now reads `retry <attempt>/<max> in <seconds>` as intended
- `TestDiagnosticFiltering` / `TestDiagnosticWriteFailureKeepsPrimaryOutput` — added `Logger.Close()` method that flushes and closes the underlying file descriptor; tests now call `Close()` so the primary log file handle is released before `t.TempDir` cleanup on Windows

---

## [2.0.0] — 2026-10-08

### Added
- **Projects** — create, update, delete projects with scope tracking and pinning
- **Project Facts** — structured key/value evidence store per project with confidence levels
- **Attack Chain** — graph-based finding visualizer per project (nodes + edges, SVG render, risk scoring, PNG/SVG export)
- **Batch Task Queues** — multi-intent sequential agent execution with real-time progress tracking
- **Human-in-the-Loop (HITL) Approvals** — operator sign-off queue with approve/reject decisions; auto-polls every 8 s
- **Conversations** — conversation archive with message replay and role-coloured message history
- **Workflows** — graph-based workflow engine (agent/tool/condition/approval/output node types) with run history
- **MCP Server Management** — register/delete external MCP servers; circuit-breaker reset endpoint
- **C2 Framework** — listeners, sessions, payloads, task dispatch, event bus, Malleable profile management
- **Multi-agent orchestration** — deep / plan-execute / supervisor modes via Eino ADK
- **WebShell terminal** — browser-based shell with cross-locale Windows directory output support
- **Knowledge base (RAG)** — document ingest, chunking, keyword + vector retrieval
- **Asset management** — bilingual CSV/Excel import with flexible column header matching
- **Vulnerability tracking** — full CRUD with severity, location, and attack-chain linkage
- **HITL audit agent** — autonomous review agent with approve/reject decision recording
- **Token usage tracking** — per-turn token counts and cost estimation
- **Tool guard** — regex-based tool call blocking with atomic policy management
- **Security headers** — CORS, CSP, rate-limit, login brute-force protection
- **TLS bootstrap** — auto self-signed cert generation on first run
- **i18n** — English, Simplified Chinese, Russian UI with live language switching
- **Full CI** — Go 1.26 + Node 22 matrix, per-package tests, CGO-enabled integration tests

### Changed
- Complete UI overhaul — dark design system, React 18 SPA with 15+ pages
- All frontend user-visible strings, comments, and labels fully in English
- Sidebar nav expanded; project-aware agent chat with folder structure
- Role filenames normalised for cross-platform compatibility
- `config.example.yaml` version field bumped to `v2.0.0`

### Fixed
- CI failures on Go 1.26 — corrected test session/token alignment
- ELK layout initialisation guard preventing crash on missing library
- Attack chain cross-tab rendering isolation (chat-switch race condition)
- `Processing...` placeholder persisting incorrectly across page refreshes

---

## [0.12.0] — 2025 — React 18 SPA
Initial 10-page frontend: Login, Dashboard, Agent (WebSocket), Assets, Vulnerabilities, Knowledge, Audit, Users, Roles, Change Password.

## [0.11.0] — Entry point + consent gate
ASCII disclaimer gate, config fallback, graceful signal shutdown.

## [0.10.0] — App wiring
Gin router, system role seeding (admin/operator/analyst/viewer), admin bootstrap, optional TLS.

## [0.9.0] — HTTP handlers
Auth, users, roles, assets, vulnerabilities, audit, agent, knowledge handlers.

## [0.8.0] — Knowledge base
RAG scaffold: document ingest, chunking, keyword search, vector retrieval slot.

## [0.7.0] — Agent runner
Single/plan-execute/supervisor modes, StepEvent streaming, LLM slot ready (rule-based stub).

## [0.6.0] — MCP tool registry
Worker pool, output size cap, tool-class guard, 3 built-in recon tools: `subdomain_enum`, `http_probe`, `dns_lookup`.

## [0.5.0] — Auth
JWT HS256, bcrypt cost=12, session table, forced password change on first login, token revocation.

## [0.4.0] — Database schema
Full SQLite schema (WAL mode): users, roles, sessions, audit, assets, vulnerabilities, tool_executions, documents, projects, attack chain, batch queues, HITL, conversations, workflow definitions and runs.

## [0.3.0] — Config loader
YAML config with `${ENV_VAR}` expansion and safe defaults.

## [0.2.0] — Project bootstrap
`go.mod`, `.gitignore`, `README.md`, `config.example.yaml`, directory skeleton.

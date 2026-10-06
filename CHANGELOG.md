# Changelog

All notable changes to Kestrel are documented here.
Format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

---

## [Unreleased] — under development

### Added
- **Projects** — create, update, delete projects with scope tracking and pinning
- **Project Facts** — structured key/value evidence store per project with confidence levels
- **Attack Chain** — graph-based finding visualizer per project (nodes + edges, SVG render, risk scoring)
- **Batch Task Queues** — multi-intent sequential agent execution with real-time progress tracking
- **Human-in-the-Loop (HITL) Approvals** — operator sign-off queue with approve/reject decisions; auto-polls every 8 s
- **Conversations** — conversation archive with message replay and role-colored message history
- **Workflows** — graph-based workflow engine (agent/tool/condition/approval/output node types) with run history
- **MCP Server Management** — register/delete external MCP servers; circuit-breaker reset endpoint
- **Workflow HTTP handler** — full CRUD + async run + run-status polling
- **Database: `workflows.go`** — `CreateWorkflowDefinition`, `GetWorkflowDefinition`, `UpdateWorkflowDefinition`, `DeleteWorkflowDefinition`
- **App wiring** — all new handlers registered in Gin router; orphaned-execution reconciliation on startup
- **Frontend navigation** — 6 new pages added to sidebar and React Router: Projects, Conversations, Batch Tasks, Workflows, Approvals
- **`api.js`** — 30+ new typed API call helpers covering all new endpoints

### Changed
- Sidebar nav expanded from 8 to 13 entries, with `overflowY: auto` to handle tall viewports
- Knowledge handler field renamed internally (`knowledge` → `kb`) for consistency

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

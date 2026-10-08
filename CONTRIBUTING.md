# Contributing to Kestrel

Thank you for your interest. This document describes how to contribute code, report issues, and understand the project's scope constraints.

---

## Scope constraint (non-negotiable)

Kestrel is scoped to **governance, workflow, and recon** operations only.

The following tool classes are **explicitly excluded** from the default recipe set and must never be added to built-in MCP tools, agent prompts, or workflow templates:

- Exploitation frameworks (remote code execution, privilege escalation)
- Credential dumping or pass-the-hash tooling
- Remote-shell / command-and-control (C2) infrastructure
- WebShell generation or deployment
- Automated exploit chaining

If you need higher-risk tool classes for a specific authorised deployment, that is a deliberate per-deployment decision made with your own controls in place — not something contributed to this repository.

---

## Development setup

```bash
# Backend
go build ./...
go test ./...

# Frontend
cd web
npm install
npm run dev    # dev server with HMR at http://localhost:5173
npm run build  # production build → web/dist/
```

### Configuration

Copy `config.example.yaml` to `config.yaml` and adjust. Secrets can be supplied via environment variables using `${VAR_NAME}` syntax in the YAML.

### Running locally

```bash
go run ./cmd/server          # HTTP on :8080
go run ./cmd/server --https  # HTTPS with self-signed cert on :8443
```

On first start, the admin credentials are printed to stdout. Change them immediately.

---

## Project structure

```
cmd/server/         — entry point (consent gate, config, graceful shutdown)
internal/
  app/              — Gin router, TLS, system seeding
  agent/            — single/plan-execute/supervisor runner
  auth/             — JWT + bcrypt + session management
  config/           — YAML loader with env-var expansion
  database/         — SQLite CRUD (WAL mode)
  handler/          — HTTP handlers per domain
  knowledge/        — RAG scaffold (ingest, chunk, search)
  logger/           — zap logger setup
  mcp/              — MCP registry, worker pool, built-in recon tools
  middleware/        — auth, audit context, password-change guard
  workflow/         — graph-based workflow engine
web/
  src/pages/        — React 18 SPA pages
  src/api.js        — typed fetch wrapper
```

---

## Conventions

- **Go**: `gofmt`, `go vet`, standard library preferred over heavy deps
- **Commits**: conventional commits — `feat:`, `fix:`, `chore:`, `docs:`, `test:`
- **API changes**: update `web/src/api.js` (React SPA) or `web/static/js/` (legacy assets) and document in `CHANGELOG.md`
- **New DB tables**: add DDL in `internal/database/schema.go`, CRUD in a matching `<domain>.go` file
- **Tests**: each new package should have at minimum a smoke test
- **No audit-log mutation**: the `audit_logs` table is append-only — never add UPDATE/DELETE paths for it

---

## Pull request checklist

- [ ] `go build ./...` passes with no warnings
- [ ] `go vet ./...` passes clean
- [ ] `npm run build` (in `web/`) produces no errors
- [ ] No new dependencies added without justification in the PR description
- [ ] `CHANGELOG.md` updated under `[Unreleased]`
- [ ] No exploitation, C2, credential-dumping, or WebShell tooling introduced

---

## Reporting issues

Open a GitHub issue describing:

1. What you expected to happen
2. What actually happened
3. Steps to reproduce
4. Go version, OS, SQLite driver version

Security-sensitive findings: do not open a public issue. Email the maintainer directly or use GitHub's private security-advisory feature.

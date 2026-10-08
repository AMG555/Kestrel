# ⚔ Kestrel — AI-Native Security Operations Platform

<div align="center">

![Status](https://img.shields.io/badge/status-under%20development-orange)
![Go](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go)
![License](https://img.shields.io/badge/license-Apache%202.0-blue)
![CI](https://github.com/AMG555/Kestrel/actions/workflows/ci.yml/badge.svg)

**A production-grade, Go-based security operations platform covering task orchestration, RBAC, audit/evidence tracking, asset management, multi-provider LLM integration, and recon-tool automation via MCP.**

> ⚠ **AUTHORIZED USE ONLY** — Use Kestrel only on systems you own or are explicitly authorized to test. All tool executions are logged.

</div>

---

## Screenshots

> UI screenshots coming soon — contributions welcome. See [`docs/screenshots/`](docs/) for the naming convention.

---

## Features

| Domain | Capability |
|--------|-----------|
| 🤖 **AI Orchestration** | Single / Plan-Execute / Supervisor agent modes; real LLM integration (OpenAI, Anthropic, Ollama) with rule-based fallback |
| 🔧 **MCP Tool Registry** | 7 built-in read-only recon tools; worker pool; output caps; per-role allowlists |
| ✋ **Human-in-the-Loop** | DB-backed approval queue; agents block on `require_approval` mode until operator decides |
| 🔐 **RBAC & Auth** | JWT HS256, bcrypt cost=12, forced password change, session revocation, 4 built-in system roles |
| 📋 **Audit Logging** | Append-only audit table; never modifiable or deletable |
| 📁 **Projects** | Scoped work items with facts, attack chains, stats, and linked vulnerabilities |
| 🕸 **Attack Chain** | Per-project DAG visualizer (SVG, layered layout, risk scoring) |
| ⚙ **Batch Tasks** | Multi-intent sequential agent execution with live progress |
| 🔀 **Workflows** | Graph-based automation engine (agent / tool / condition / approval / output node types) |
| 💬 **Conversations** | Full message archive with role-colored replay |
| 📦 **Assets** | Dedup/normalize on ingest; CSV export |
| 🛡 **Vulnerabilities** | Severity lifecycle, asset linking, filtering |
| 📚 **Knowledge Base** | RAG pipeline (chunk/ingest/keyword search; vector slot ready for embedding model) |
| 📊 **Reports** | Markdown, JSON, and CSV project report export |
| 🔒 **TLS** | Self-signed cert for local dev; configurable cert/key paths for production |

---

## Quick Start

```bash
# 1. Install Go 1.26+ and Node 22+

# 2. Clone
git clone https://github.com/AMG555/Kestrel.git
cd Kestrel

# 3. Build everything
make all

# 4. Configure
cp config.example.yaml config.yaml
# Edit config.yaml — set jwt_secret or use ${JWT_SECRET} env var

# 5. Run
./build/kestrel          # HTTP on :8080
./build/kestrel --https  # HTTPS with self-signed cert on :8443
```

Or use `go run` directly:
```bash
cd web && npm install && npm run build && cd ..
go run ./cmd/server
```

On first startup the admin credentials are printed to the console. Sign in and change the password immediately.

---

## LLM Configuration

Kestrel ships in **rule-based stub mode** by default — no API key required. To enable real AI:

```yaml
# config.yaml
ai:
  default_channel: openai
  channels:
    openai:
      provider: openai
      api_key: ${OPENAI_API_KEY}
      model: gpt-4o
    anthropic:
      provider: anthropic
      api_key: ${ANTHROPIC_API_KEY}
      model: claude-3-5-sonnet-20241022
    local:
      provider: ollama
      base_url: http://localhost:11434
      model: llama3.2
```

---

## Architecture

```
Kestrel/
├── cmd/server/             Entry point (consent gate, config, graceful shutdown)
├── internal/
│   ├── agent/              ReAct loop — LLM-driven or rule-based fallback
│   ├── app/                Gin router, TLS, system role seeding
│   ├── auth/               JWT + bcrypt + session management
│   ├── config/             YAML loader with ${ENV_VAR} expansion
│   ├── database/           SQLite (WAL) — all domain CRUD
│   ├── handler/            HTTP handlers per domain
│   ├── knowledge/          RAG pipeline (ingest, chunk, keyword/vector search)
│   ├── llm/                Multi-provider LLM client (OpenAI, Anthropic, Ollama, stub)
│   ├── logger/             Structured zap logger
│   ├── mcp/                Tool registry + 7 built-in recon tools
│   ├── middleware/         Auth, audit context, forced-change guard
│   ├── report/             Markdown / JSON / CSV report generator
│   └── workflow/           Graph-based workflow engine
└── web/
    ├── src/                React 18 SPA source (compiled to web/dist/, embedded in Go binary)
    │   ├── pages/          16 pages: Dashboard, Agent, Projects, AttackChain,
    │   │                              BatchTasks, Workflows, HITL, Conversations,
    │   │                              Assets, Vulns, Knowledge, Audit,
    │   │                              Users, Roles, ChangePassword
    │   ├── App.jsx         Router + Auth context + Sidebar layout
    │   └── api.js          60+ typed fetch helpers + WebSocket
    ├── static/             Go-embedded legacy JS/CSS assets
    └── templates/          Go-embedded HTML templates
```

---

## Built-in Recon Tools (Read-Only)

| Tool | Description |
|------|-------------|
| `subdomain_enum` | Passive subdomain enumeration via crt.sh CT logs |
| `http_probe` | HTTP/HTTPS service probing (status, server header, redirect) |
| `dns_lookup` | DNS record lookup (A, AAAA, MX, TXT, NS, CNAME) |
| `whois_lookup` | RDAP/WHOIS lookup for domains and IP addresses |
| `ssl_cert_check` | TLS certificate inspection (expiry, SANs, issuer, chain) |
| `tech_fingerprint` | Passive technology detection from HTTP headers + HTML |
| `port_scan` | TCP connect scan across common ports (up to 200 ports, 50 concurrent) |

> Exploitation, credential-dumping, and remote-shell tool classes are **deliberately excluded** from the default tool set.

---

## System Roles

| Role | Permissions | HITL Mode |
|------|-------------|-----------|
| `admin` | Full access (`*`) | Auto |
| `operator` | Read/write assets + vulns, run agent + tools | Auto |
| `analyst` | Read assets + vulns, run agent + tools | Require approval |
| `viewer` | Read-only | Require approval |

---

## API Reference

<details>
<summary>Click to expand full API surface</summary>

```
# Auth
POST   /api/auth/login
POST   /api/auth/logout
POST   /api/auth/change-password
GET    /api/auth/me

# System
GET    /api/system/info
GET    /api/dashboard/stats

# Users & Roles
GET|POST           /api/users
GET|PATCH|DELETE   /api/users/:id
POST|DELETE        /api/users/:id/roles/:role_id
GET|POST           /api/roles
GET|PATCH|DELETE   /api/roles/:id

# Projects
GET|POST           /api/projects
GET|PATCH|DELETE   /api/projects/:id
GET|POST           /api/projects/:id/facts
DELETE             /api/projects/:id/facts/:fact_id
GET                /api/projects/:id/attack-chain
POST               /api/projects/:id/attack-chain/nodes
POST               /api/projects/:id/attack-chain/edges
GET                /api/projects/:id/report?format=markdown|json|csv

# Assets & Vulnerabilities
GET|POST           /api/assets
GET|DELETE         /api/assets/:id
GET|POST           /api/vulnerabilities
GET|PATCH|DELETE   /api/vulnerabilities/:id

# Audit
GET                /api/audit

# Tools & Agent
GET                /api/tools
POST               /api/tools/:name/execute
POST               /api/agent/run
WS                 /api/agent/stream

# Knowledge
GET                /api/knowledge/documents
POST               /api/knowledge/ingest
POST               /api/knowledge/query
DELETE             /api/knowledge/documents/:id

# Batch Tasks
GET|POST           /api/batch/queues
GET|DELETE         /api/batch/queues/:id
POST               /api/batch/queues/:id/run
POST               /api/batch/queues/:id/cancel

# HITL Approvals
GET                /api/hitl/pending
POST               /api/hitl/:id/decide

# Conversations
GET|POST           /api/conversations
PATCH|DELETE       /api/conversations/:id
GET                /api/conversations/:id/messages

# MCP Servers
GET|POST           /api/mcp/servers
DELETE             /api/mcp/servers/:id
POST               /api/mcp/servers/:id/reset-circuit

# Workflows
GET|POST           /api/workflows
GET|PATCH|DELETE   /api/workflows/:id
POST               /api/workflows/:id/run
GET                /api/workflows/runs/:run_id
```

</details>

---

## Development

```bash
make test         # Run all tests (CGO_ENABLED=1, requires gcc)
make test-short   # Skip network-dependent tests
make vet          # go vet
make fmt          # go fmt
make dev          # air hot-reload backend
make web-dev      # Vite dev server (HMR)
make clean        # Remove build artifacts
```

> **Windows note:** Database tests require CGO. Install [TDM-GCC](https://jmeubank.github.io/tdm-gcc/) and run `$env:CGO_ENABLED=1; go test ./...`. Shell tests in `internal/security` are Linux-only and expected to fail on Windows.

---

## Configuration Reference

```yaml
server:
  host: 127.0.0.1
  port: 8080
  tls_auto_self_sign: false  # set true for HTTPS with self-signed cert

auth:
  jwt_secret: ${JWT_SECRET}  # auto-generated if blank
  session_duration_hours: 12

ai:
  default_channel: stub  # stub | openai | anthropic | ollama
  channels: {}

mcp:
  worker_pool_size: 4
  call_timeout_seconds: 120
  output_cap_bytes: 102400

hitl:
  enabled: true
  default_mode: auto  # auto | require_approval
  approval_timeout_seconds: 300

knowledge:
  enabled: false
  chunk_size: 512
  chunk_overlap: 64
  top_k: 5
```

---

## License

Apache 2.0 — see [LICENSE](LICENSE).

---

<div align="center">
<sub>⚠ For authorized security operations only. All actions are logged.</sub>
</div>

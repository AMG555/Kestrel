# Kestrel — AI-native Security Operations Platform

> **Status: Under Development**

Kestrel is a Go-based, AI-native security operations platform covering task orchestration, RBAC, audit/evidence tracking, asset management, and recon-tool integration via MCP.

**AUTHORIZED USE ONLY** — Use Kestrel only on systems you own or are explicitly authorized to test.

---

## Architecture

```
kestrel/
├── cmd/server/main.go          # Entry point
├── internal/
│   ├── app/                    # Application wiring, router, seeding
│   ├── auth/                   # JWT authentication, session management
│   ├── agent/                  # Agent orchestration (single, plan-execute, supervisor)
│   ├── config/                 # YAML config loading + env-var expansion
│   ├── database/               # SQLite persistence (WAL, schema, CRUD)
│   ├── handler/                # HTTP/WebSocket handlers
│   ├── logger/                 # Structured zap logger
│   ├── mcp/                    # MCP tool registry + 3 built-in recon tools
│   └── middleware/             # Auth middleware, RBAC checks
└── web/                        # React SPA (Vite)
    └── src/
        ├── pages/              # Dashboard, Agent, Assets, Vulns, Audit, Users, Roles
        ├── App.jsx             # Router + Auth context + Layout
        ├── api.js              # Fetch wrapper + WebSocket helper
        └── index.css           # Design system
```

## Quick Start

```bash
# 1. Install dependencies
cd web && npm install && npm run build && cd ..

# 2. Copy config
cp config.example.yaml config.yaml

# 3. Run (Go 1.22+)
go run cmd/server/main.go

# Or with HTTPS (self-signed):
go run cmd/server/main.go --https
```

On first startup, an auto-generated `admin` password is printed to the console. Sign in and change it immediately.

## Iterative Build Plan

| Layer | Status |
|-------|--------|
| Data model + schema | ✅ |
| Config + YAML loading | ✅ |
| Auth (JWT + bcrypt + forced change) | ✅ |
| RBAC (users, roles, permissions) | ✅ |
| Audit logging (append-only) | ✅ |
| Asset management | ✅ |
| Vulnerability tracking | ✅ |
| MCP tool registry + 3 recon tools | ✅ |
| Agent orchestration (single + plan-execute + supervisor) | ✅ |
| WebSocket streaming | ✅ |
| HTTPS + self-signed TLS | ✅ |
| React SPA (dashboard, agent, assets, vulns, audit, RBAC) | ✅ |
| Knowledge base (RAG stub) | 🔲 |
| LLM integration (replace rule-based stub) | 🔲 |

## Built-in Recon Tools (Read-Only)

| Tool | Description |
|------|-------------|
| `subdomain_enum` | Passive subdomain enumeration via crt.sh certificate transparency |
| `http_probe` | HTTP/HTTPS service availability probing |
| `dns_lookup` | DNS record lookups (A, AAAA, MX, TXT, NS, CNAME) |

Exploitation, credential-dumping, and remote-shell tool classes are **deliberately excluded** from the default tool set.

## System Roles

| Role | Permissions | HITL |
|------|-------------|------|
| `admin` | Full access | Auto |
| `operator` | Read/write assets+vulns, run agent + tools | Auto |
| `analyst` | Read assets+vulns, run agent + tools | Require approval |
| `viewer` | Read only | Require approval |

## API

```
POST   /api/auth/login
POST   /api/auth/logout
POST   /api/auth/change-password
GET    /api/auth/me

GET    /api/dashboard/stats
GET    /api/system/info

GET    /api/users              POST /api/users
GET    /api/users/:id          PATCH /api/users/:id  DELETE /api/users/:id
POST   /api/users/:id/roles    DELETE /api/users/:id/roles/:role_id

GET    /api/roles              POST /api/roles
GET    /api/roles/:id          PATCH /api/roles/:id  DELETE /api/roles/:id

GET    /api/assets             POST /api/assets
GET    /api/assets/:id         DELETE /api/assets/:id

GET    /api/vulnerabilities    POST /api/vulnerabilities
GET    /api/vulnerabilities/:id  PATCH /api/vulnerabilities/:id  DELETE /api/vulnerabilities/:id

GET    /api/audit

GET    /api/tools              POST /api/tools/:name/execute
POST   /api/agent/run
WS     /api/agent/stream
```

## License

Apache 2.0

---

> CyberStrikeAI inspired this project's architecture. Kestrel is an independent implementation scoped to governance, RBAC, audit, asset management, and read-only recon — without C2, WebShell, or exploit-chaining capabilities.

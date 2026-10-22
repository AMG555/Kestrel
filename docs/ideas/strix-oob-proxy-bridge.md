# Kestrel OAST (Interactsh) & Caido Proxy Bridge (Strix Synergies)

## Problem Statement
Traditional AI security agents frequently hallucinate out-of-band vulnerability findings (blind SSRF, blind RCE, blind SQLi, XXE) or rely on brittle, plain-text services like `dnslog.cn` that drop callbacks, leak payloads over unencrypted HTTP, and cause race conditions across concurrent sessions. Additionally, auditors lack visibility into what HTTP traffic agents generate and cannot easily replay or mutate requests through an interactive auditing proxy.

## Architecture & Implementation

### 1. Interactsh Out-of-Band Listener (`tools/interactsh.yaml`, `tools/scripts/interactsh_client.py`)
- **Protocol**: ProjectDiscovery Interactsh protocol (compatible with public servers `oast.pro`, `oast.live`, `oast.site` or custom self-hosted servers).
- **Cryptography**:
  - RSA 2048-bit keypair per session.
  - Public key registered with server.
  - Server encrypts AES-256 session key with RSA OAEP-SHA256.
  - Interaction records encrypted with AES-CTR (16-byte IV prefix).
- **Isolation**: Each registration uses an isolated `correlation-id` and session file in temp directory, avoiding race conditions between parallel scan agents.
- **Protocols Supported**: DNS (A, AAAA, TXT, CNAME, etc.), HTTP/HTTPS (full raw headers + body), SMTP, LDAP.

### 2. Caido Proxy Integration (`tools/caido.yaml`, `tools/scripts/caido_client.py`)
- **GraphQL Integration**: Queries Caido's GraphQL backend at `http://127.0.0.1:8080/graphql` to inspect intercepted request history and filter by target domain or HTTP status.
- **Inspection**: Retrieves full raw HTTP request and response structures by ID.
- **Replay**: Dispatches requests through Caido's upstream proxy (`http://127.0.0.1:8080`), logging all AI test probes directly in the operator's Caido timeline.

### 3. Windows Compatibility Layer (`internal/security/executor.go`)
- Transparent binary resolution (`resolveExecutable`): automatically resolves `python3` to `python` on Windows hosts when `python3` is not present in `%PATH%`, ensuring all Python-based security tool recipes execute out-of-the-box.

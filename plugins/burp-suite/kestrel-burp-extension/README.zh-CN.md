## Kestrel Burp Suite Extension

### Features

- Configure **Host, Port, and Password** in the `Kestrel` tab inside Burp
- Click **Validate**:
  - Calls `POST /api/auth/login` to exchange the password for a token
  - Calls `GET /api/auth/validate` to verify the token
  - On success the token is stored in plugin memory (valid for the current Burp session)
- Right-click any HTTP request → **Send to Kestrel (stream test)**:
  - Opens a send dialog where you can configure for **the current traffic**:
    - **Project** (`GET /api/projects`; the `projectId` is bound when a new conversation is created)
    - **Role** (`GET /api/roles`)
    - **Conversation mode** (Eino Single / Deep / Plan-Execute / Supervisor; chosen per request and remembered)
    - **Test instruction** (editable prompt prefix)
  - Selections are remembered from the previous request for convenient consecutive testing of similar traffic
- **Test history sidebar (searchable)**: each Send adds a record for easy review and comparison
- **Output area**: `Progress` (collapsible) + `Final Response` (main area)
- **Markdown rendering**: final output can be rendered as rich text in the Output main area (toggleable)
- **Request / Response review**: right-side tabs let you view the raw captured request/response
- **Stop cancel**: after a task creates a conversation, call `/api/agent-loop/cancel` to stop the current session task

### Build (recommended — no Gradle/Maven required)

> For regular users: distribute the **pre-built jar** and users load it in Burp — **no compilation needed**.

#### Method A (recommended, general): build with Maven (no need to know where Burp is)

Suitable for: developers / CI packaging once, distributing to all users.

Requirements:

- JDK 11+
- Maven (downloads `burp-extender-api` dependency from Maven Central)

Build:

```bash
cd plugins/burp-suite/Kestrel-burp-extension
./build-mvn.sh
```

Output:

- `dist/Kestrel-burp-extension.jar`

#### Method B (offline): pure JDK build (requires the Burp API jar)

- JDK 11+
- Burp Extender API jar (from your Burp installation directory)

#### Steps

1) Create `lib/` in the plugin directory and copy `burp-extender-api.jar` into it:

```bash
cd plugins/burp-suite/Kestrel-burp-extension
mkdir -p lib
# Copy the Burp API jar here, e.g.:
# cp "/path/to/burp-extender-api.jar" lib/
```

2) One-command build:

```bash
cd plugins/burp-suite/Kestrel-burp-extension
./build.sh
```

Output:

- `dist/Kestrel-burp-extension.jar`

### Loading in Burp Suite

- Burp Suite → **Extensions** → **Installed** → **Add**
- Extension type: **Java**
- Select `dist/Kestrel-burp-extension.jar`

### Usage

1) Open the `Kestrel` tab at the top of Burp
2) Fill in:
   - **Host**: e.g. `127.0.0.1`
   - **Port**: e.g. `8080`
   - **HTTPS**: checked by default (for `tls_enabled` / self-signed certificates in `config.yaml`); the plugin trusts local self-signed certificates automatically, no import required
   - **Password**: your Kestrel login password (corresponds to `auth.password` on the server)
3) Click **Validate**
   - Success: status shows `OK (token saved)`
   - Failure: status shows the error reason (e.g. wrong password, server unreachable, 401/403, etc.)
4) Select an HTTP message in Burp's Proxy / HTTP history / Repeater list
5) Right-click → **Send to Kestrel (stream test)**
6) In the dialog, choose **project, role, and conversation mode** as needed, edit the test instruction, and confirm
7) Each Send adds a "test record" (request title + mode/role + status) in the left sidebar of the `Kestrel` tab; click the record to view the streaming output on the right

### Troubleshooting

- **Validate fails / 401**
  - Confirm the password is correct (server-side `auth.password`)
  - Confirm the IP/port is reachable (e.g. the browser can open `https://IP:PORT/`)
  - Check **HTTPS** when the server has TLS enabled (checked by default); self-signed certificates require no manual import
  - Uncheck **HTTPS** if using plain HTTP

- **"Multi-agent not enabled" shown after selecting Multi Agent**
  - Enable it server-side: set `multi_agent.enabled: true` in `config.yaml`
  - Restart the server (or apply the config dynamically per your project's procedure)

- **No streaming output after right-click Send**
  - Confirm Validate has been completed (token obtained)
  - Confirm Burp can reach Kestrel (network / proxy / firewall)
  - The server's streaming endpoint is SSE; the plugin parses `data: {json}` lines. Middleware buffering may reduce real-time responsiveness

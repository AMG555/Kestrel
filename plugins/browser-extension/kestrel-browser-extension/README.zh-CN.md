## Kestrel Browser Extension

**Current version: 0.3.10**

Chrome / Edge (Chromium) DevTools extension: capture **Network** traffic in the developer tools and send it to Kestrel for AI-assisted security testing. Feature-aligned with the Burp Suite plugin, with additional performance and usability optimisations for production scenarios.

---

### Quick start

1. `chrome://extensions/` → Developer mode → **Load unpacked**
2. Select directory: `plugins/browser-extension/Kestrel-browser-extension/`
3. Open the target page → **F12** → top tab **Kestrel**
4. Fill in Host / Port / Password → **Validate** (first time will request permission to access the server address)
5. Select a captured request on the left → **Send** → view AI results in **Output**

Click the browser toolbar icon to see the **read-only connection status**; all configuration and operations are done inside the DevTools panel.

---

### UI layout

```
┌─ Connection bar (can be collapsed after Validate) ──────────────┐
│ Logo │ https://host:port │ Connection settings │ ● OK           │
├─ Action bar ────────────────────────────────────────────────────┤
│ Send │ Latest XHR │ Stop │ Copy │ Clear │ ● Capturing/○ Paused  │
│                    XHR/Fetch only │ Debug │ Markdown            │
├──────────────┬──────────────────────────────────────────────────┤
│ Test History │ Output │ Request │ Response                      │
│ Captured Req │ Progress + Final Response                        │
└──────────────┴──────────────────────────────────────────────────┘
```

| Area | Description |
|------|-------------|
| **Connection bar** | Host, Port, HTTPS, Password, Validate; collapses to `https://host:port` summary after success |
| **Test History** | Up to 50 Send records; revisit Progress / Final for each |
| **Captured Requests** | Current tab capture list, up to 200 entries, with search |
| **Output** | Default tab: streaming Progress + Final Response |
| **Request / Response** | View raw HTTP/1.1 format for the selected captured entry |

---

### Features

#### Capture

- **Background hub**: `devtools.js` listens to Network → `service-worker` queue → Panel subscribes
- Defaults to **XHR/Fetch only** (can be disabled to capture more types)
- Static-asset URL / MIME **pre-filter**: response body is not read until a match is made
- **● Capturing / ○ Paused**: zero overhead when paused; existing list can still be Sent
- Per-entry truncation: request body **64 KB**, response **4 KB**

#### HTTP display and AI prompt

- **Storage**: raw HAR kept in memory (including HTTP/2 pseudo-headers such as `:method`)
- **Display / Prompt**: normalised to **HTTP/1.1** (consistent with the Burp plugin)

```http
GET /api/foo HTTP/1.1
Host: example.com
Cookie: ...
```

#### Sending to Kestrel

- Pop-up selection: **project / role / conversation mode** (dynamic API) + test instruction
- Supports **Eino Single**, **Deep**, **Plan-Execute**, **Supervisor**
- **Latest XHR**: one click selects the most recent API request and opens the Send dialog
- **Stop**: cancels local SSE + calls server-side `/api/agent-loop/cancel`

#### Streaming output

- Progress log cap: **512 KB** (truncated if exceeded)
- **Final Response is not truncated** (current in-progress test)
- Historical run Final soft-truncated at **100 KB** after switching away
- **Markdown**: plain text during streaming; rendered via `requestIdleCallback` after completion; degrades to plain text above **100 KB**
- **Copy**: copies current Request / Response / Final

#### Security and permissions

- Token stored in **chrome.storage.session** (expires when browser closes)
- After login, `expires_at` is saved; status bar shows **remaining time** (e.g. `OK · 11h 30m left`)
- **No automatic token renewal**: re-validate after expiry (requires Password)
- Local expiry check (30 s) + server-side `/api/auth/validate` probe (same interval; immediate on DevTools panel focus)
- Shows **Cannot connect to server** when unreachable; shows **Server restarted or token expired** when token is invalid after restart
- **401/403** clears token automatically and expands the connection bar
- Token validity is checked proactively before Send
- **optional_host_permissions**: granted on demand during Validate
- Permission request is bound to the Validate click event; only requests the current Kestrel service origin; no repeat prompts for already-authorised addresses

---

### Buttons and options

| Control | Function |
|---------|----------|
| **Validate** | Login and verify token; click again while in progress to cancel |
| **Connection settings / Collapse** | Expand or collapse the Host/Port/Password form |
| **Send** | Send the selected captured entry to Kestrel |
| **Latest XHR** | Select the most recent XHR/Fetch entry and Send |
| **Stop** | Stop the current AI stream (local + server-side) |
| **Clear Output** | Clear Progress / Final for the current run |
| **● Capturing / ○ Paused** | Enable or pause Network capture |
| **XHR/Fetch only** | Capture API-type requests only |
| **Debug events** | Show additional SSE events in Progress |
| **Markdown** | Render rich text after Final completes |
| **Clear All** | Clear Test History |
| **Clear** | Clear the current tab's capture list |

---

### Data and memory (no unbounded growth)

| Data | Limit | Location | Cleared when |
|------|-------|----------|--------------|
| Captured requests | 200 / tab | Background + Panel memory | Oldest dropped on overflow; manual Clear |
| Tab capture slots | 20 tabs | Background memory | Non-current tab dropped on overflow |
| Test history | 50 entries | Panel memory | Oldest dropped on overflow; Clear All |
| Progress | 512 KB / run | Panel memory | Truncated on overflow |
| Final (in progress) | No hard limit | Panel memory | — |
| Final (historical) | 100 KB soft cap | Panel memory | When switching to another run |
| Config + Token | Very small | chrome.storage | Manual config change |

- Close **DevTools** → Panel memory cleared
- Close **browser** → Session token expires
- Service Worker recycled → Background capture queue cleared

---

### Performance notes

| Scenario | Impact |
|----------|--------|
| DevTools closed | **No impact** (not listening to Network) |
| DevTools open + capture paused | **Virtually no impact** |
| DevTools open + capturing + XHR only | Minor overhead for matched requests only |
| High-traffic SPA | Recommend keeping **XHR/Fetch only** enabled; click **Paused** when not needed |

Optimisations already applied: filter memory cache, no body read for static assets, incremental list insertion, debounced search, rAF-throttled streaming UI.

---

### FAQ

**Extension shows `chrome.runtime.connect` undefined after update?**
The old DevTools panel context becomes invalid after an extension reload. Fix: **Close DevTools → Reload the extension → Open F12 again**.

**Will the token auto-refresh?**
**No automatic renewal** (no refresh token). The plugin saves `expires_at` and shows the remaining time; it checks the server every 30 s and immediately on DevTools focus. After a server restart the session is cleared and you will be prompted to Validate again.

**Status still shows OK after server restart?**
Since v0.3.7 it probes `/api/auth/validate` every 30 s; shows a yellow warning if unreachable, clears token and expands the connection bar if token is invalid. After reloading the extension, close DevTools and reopen F12.

**Why did Request once contain `:authority` and `:method`?**
HTTP/2 pseudo-headers. Display and AI prompt have been normalised to HTTP/1.1; the raw HAR is still kept in the in-memory entry.

**Is the localhost CORS error in Console caused by the extension?**
No. That is the page itself making requests to a local service blocked by the browser, and is unrelated to the extension.

**Validate shows `cross-origin request denied`?**
Upgrade and restart Kestrel. The new server automatically recognises well-formed Chrome/Edge extension Origins, so no need to copy the extension ID or configure a CORS allowlist. The first Validate will still request browser permission to access the target service address.

**Validate asks permission to access the Kestrel service?**
Allow access in the browser permission prompt. The extension only requests the filled-in service origin on demand; no site-wide access is needed. If the prompt does not appear, reload the extension in `chrome://extensions/`, fully close DevTools, then reopen it and click Validate.

**HTTPS shows connection failed, but Burp works fine?**
The Burp plugin trusts self-signed certificates; browser extensions cannot bypass Chromium's TLS validation. Open the service URL in the browser first and trust the certificate. For production use a trusted certificate with the service IP/domain in the SAN.

**Will many Test History entries obscure Captured Requests?**
No. The history section occupies at most **42%** of the sidebar height; overflow scrolls within that area, and the capture section takes the remaining space.

**Will it slow down web pages?**
No impact during normal browsing (DevTools closed). Use **Paused** to fully stop capture during debugging.

---

### Popup vs DevTools responsibilities

| Location | Purpose |
|----------|---------|
| **DevTools panel** | Connect, Validate, capture, Send, Output (primary workspace) |
| **Extension popup** | Read-only connection status + version number + guidance to open DevTools |

The full configuration form is not duplicated in the popup to avoid drift from the main workflow.

---

### Packaging

```bash
bash plugins/browser-extension/Kestrel-browser-extension/package.sh
# → dist/Kestrel-browser-extension.zip
```

Icons are generated from the project root `images/logo.png`:

```bash
LOGO="images/logo.png"
ICONS="plugins/browser-extension/Kestrel-browser-extension/icons"
for size in 16 48 128; do
  sips -z $size $size "$LOGO" --out "$ICONS/icon${size}.png"
done
```

---

### Limitations

- Chrome **does not provide** a Network panel right-click menu API → use **Latest XHR** + custom list
- Firefox requires `about:debugging` for temporary loading; Token falls back to `local` when `storage.session` is unavailable
- Cannot jump directly from the popup to a specific DevTools panel (Chrome API limitation)

---

### Directory structure

```text
manifest.json                 # MV3 manifest
background/service-worker.js  # capture queue, Panel port, global toggle
devtools.js                   # Network listener (earliest filtering)
devtools.html
panel/
  panel.html / panel.js / panel.css   # main UI
popup/
  popup.html / popup.js / popup.css   # read-only status
lib/
  auth-session.js     # token expiry detection and remaining-time display
  api.js              # login, SSE, project/role API
  storage.js          # config + session token + expires_at
  capture.js          # HAR summary, static filtering
  http-normalize.js   # HTTP/2 → HTTP/1.1 display/prompt
  formatter.js        # toPrompt assembly
  markdown.js         # Final Markdown rendering
  catalog-cache.js    # project/role 5-minute cache
  constants.js        # limit constants
icons/                # 16 / 48 / 128
package.sh
```

---

### Comparison with the Burp plugin

| Capability | Burp plugin | Browser extension |
|------------|-------------|-------------------|
| Traffic source | Proxy history | DevTools Network |
| Connection config | In-tab | In-tab (collapsible) |
| HTTP format | HTTP/1.1 | Display/Prompt normalised to HTTP/1.1 |
| Project / role / mode | Send dialog | Send dialog |
| SSE output | Progress + Final | Progress + Final |
| Capture toggle | — | ● Capturing / ○ Paused |

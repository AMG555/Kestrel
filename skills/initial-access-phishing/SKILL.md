---
name: initial-access-phishing
description: >-
  Initial access / phishing / social engineering: credential spraying, AiTM, device code, OAuth consent phishing, payloads, vishing. Use when needing initial access, phishing, AiTM, device code, or social engineering.
metadata:
  tags: [penetration-testing, red-team]
---

## Initial Access / Phishing / Social Engineering

```
=== Initial Access / Phishing / Social Engineering (the first hop from external to internal — pre-exploitation preparation) ===
Credential spraying: weak passwords (Season+Year!/Company123) sprayed across all accounts (1-2 attempts per account to avoid lockout) | Sources: leaked databases / OSINT email format (f.last@) / default credentials
AiTM phishing (bypass MFA): evilginx3/Modlishka reverse proxy of real site → man-in-the-middle steals session cookie + token (bypasses MFA) | phishlet configures domain + certificate
Device code phishing (Azure/M365): device code flow → trick victim into entering code → obtain access/refresh token (no password or MFA needed) | TokenTactics/AADInternals
Phishing payloads: gophish to send email + landing page | payloads: lnk/iso/macro/HTA/OneNote | bypass email gateway: password-protected zip / cloud storage link / HTML smuggling
OAuth consent phishing (illicit consent): malicious app requests excessive scope (Mail.Read/offline_access) → victim consents → long-lived access token
Social engineering prerequisites: OSINT on org structure / vendors / password habits (LinkedIn/job postings/GitHub) | pretext (IT support/vendor/HR) | vishing (deepfake voice)
→ once initial access is established, pivot to `redteam-opsec` (don't get caught by EDR the moment you land), feed acquired capabilities into `capability-primitive-search` to chain further
```

---
name: redteam-opsec
description: >-
  OPSEC / covert operations discipline: IP blacklist bypass, rate/timing control, traffic obfuscation, minimal footprint, anti-forensics, progressive exposure. Use when maintaining stealth, bypassing IP bans, or planning covert red-team ops.
metadata:
  tags: [penetration-testing, red-team]
---

## OPSEC / Covert Operations Discipline (AV evasion / stability / stealth)

```
Core: getting in ≠ staying in. Being detected resets the operation to zero. Before every action, ask "what does this step look like from the defender's perspective?"
🚨IP blacklist bypass (first thing to try after being banned): X-Forwarded-For: <random IP> (when CDN trusts this header, it directly bypasses application-layer blacklists; if code:10010/blacklisted, try XFF/X-Real-IP/CF-Connecting-IP/True-Client-IP one by one)
  Verify: normal request returns "blacklisted" + your real IP → add XFF header and get normal 200 → attach XFF to all subsequent requests
  Advanced: rotate XFF value every N requests (to prevent the new IP from also being banned) | some CDNs only trust the first XFF value, others trust the last
Rate and timing: throttle scans (nuclei -rl / nmap -T2 --max-rate / ffuf -p delay) to avoid WAF bans + IDS threshold alerts | high-risk actions at low frequency + random jitter | avoid both business peak hours and late nights (blending with target's work schedule is least conspicuous)
Traffic obfuscation: mimic normal business traffic (common UA / Referer / legitimate paths) | probe defenses before heavy tools (process list / known EDR / SIEM agent fingerprints) → if present, prefer silent methods; only use automated bulk tools when no monitoring is detected
Minimal footprint: in-memory execution first, no disk writes (DDexec / memfd / reflective loading) | webshell with strong password + non-common path + disguised functionality | tunnel over 443/DNS to blend with common egress | delete tools after use (/dev/shm memory disk, leave no debris)
Anti-forensics: command history: unset HISTFILE / set +o history | selectively delete own log entries (truncating everything triggers alerts) | timestamps: touch -r <reference-file> to preserve mtime of persisted files | don't touch monitoring/audit services (stopping them is itself an alert)
Progressive exposure: passive recon (cert transparency / passive DNS / search engines / asset engines) → confirm no strong monitoring → then active scanning → last resort: exploitation and foothold. What can be obtained from public intel should never require touching the target. Before escalating each level, ask "is it worth the exposure?"
> Stealth is not obsessiveness — it is a red team's survival capability. One reckless full-port full-speed scan can zero out the entire operation.
```

---
name: capability-primitive-search
description: >-
  Capability primitives + state-space search: compose read/write/exec/ssrf and other primitives into RCE equations A-F, map low-severity findings, forward/backward search, cross-domain cashout. Use when no single RCE, chaining low-severity vulns, or deriving novel attack chains.
metadata:
  tags: [penetration-testing, red-team]
---

## Capability Primitives + State-Space Search

```
Mindset: RCE is not a single "vulnerability" — it is the emergence of a complete set of "capabilities". Even without a single high-severity bug, combining info-level / low-severity findings can yield code execution.
  Fixed mindset to break: "Scanner found no RCE/deserialization/upload → this target is un-attackable." Your job is not to "find an RCE vulnerability" — it is to "assemble the primitives required for execution."
  Abstract every Fact as a capability primitive (don't log "found vuln X", log "what capability I now have + its constraints"):
    read(path) write(path) exec(cmd) ssrf(url) sqli redirect(url) eval_expr idor(id) cred(svc,priv) coerce_auth write_acl

RCE only needs to satisfy any one equation — decompose it into obtainable primitives and assemble them:
  A. can write a file + the file is executed as code                      = RCE
  B. can control config/env + config points to your code                  = RCE
  C. can reach admin panel + panel has built-in execution functionality    = RCE (not a bug — a feature!)
  D. have credentials + service has a legitimate execution entry point     = RCE (abusing legitimate functionality)
  E. arbitrary read + read yields credentials + credentials allow login to execution point = RCE
  F. can control data + data flows into a dangerous sink (eval/template/SQL) = RCE

Low-severity → primitive mapping (translate "low-value findings" into puzzle pieces):
  Info leak (.git/backup/stack trace) → source/path/key → feed B/E/F | LFI/arbitrary-read → read config keys → E, or log poisoning → A
  SSRF (even GET only) → attack internal Redis/Consul/K8s/cloud metadata → C; cloud metadata yields temp creds → D
  Weak/default/reused creds → access a backend with task/plugin/webhook/CI functionality → C | CORS/CSRF/XSS → use admin browser to trigger execution-class features → C
  Controllable upload (even with extension restriction) → combine with path traversal/parse difference/.htaccess → A | config write → modify template/log/connection string → B
  SQLi (even read-only) → read hash/key → E, or OUTFILE → A | controllable template → SSTI → F | prototype pollution → pollute downstream properties → F

State-space search (when no ready-made chain exists, search yourself): state = current capability set, action = use capabilities to unlock new capabilities, goal = Goal.
  Forward: for each capability ask "what does it unlock?"; for each capability pair ask "what do they compose into?"
    (read+write=modify config; ssrf+internal-redis=RCE; sqli+FILE=webshell; coerce_auth+relay=domain SYSTEM; idor+mass-assign=elevate other user to admin)
  Backward (preferred when stuck): lock Goal=execute command → pick the equation closest to current state as template → identify the missing primitive and set it as sub-goal →
    which low-severity/feature/info-leak can supply it? → if none, recurse/try another equation → forward and backward meet in the middle = complete chain emerges → validate each step

Breakpoints (don't walk past these):
  · "Feature is a primitive": backend task scheduler/plugin/template editor/SQL console/file manager/import-export/webhook — legitimate features, but gaining access means you already have execution/read/write primitives. No distinction between "feature" and "vulnerability" — only "capability" matters.
  · Cross-protocol jump: SSRF using gopher/dict/file turns "HTTP only" into "attack Redis/send SMTP/read files".
  · Credential reuse is universal glue: any password/key obtained anywhere — spray it across all services by default. Lateral movement is usually faster than vertical.
  · Parse difference: upload validator / router / reverse proxy each have different understanding → bypass lives in the gap (double extension / encoding / Host obfuscation).
  · Time dimension: TOCTOU / predictable token / cache poisoning — turn "occasional" into "stable", turn "unexploitable" into "exploitable".
  · Cross-domain cashout: for every capability gained, ask "what is it worth in another domain?" — capabilities are universal currency. Web SSRF → cloud metadata → account takeover; APK hard-coded key → internal API bypassing frontend auth; supply chain → CI secrets → production access.
  · Creative mode (when known combinations are exhausted): re-examine capability boundaries (can only read /var/log? what about /proc/self/environ?) | find equivalent RCE sinks (write to crontab/.bashrc/CI config/LD_PRELOAD/authorized_keys/systemd unit = RCE) | use information (errors/timing/response length) as side channels | negate assumptions: list "things I thought were impossible" and ask "why exactly is each one impossible?"
> A derived chain is a hypothesis (record it as tentative note/chain). Only write confirmed Fact / record_vulnerability after actual execution + evidence. The entire chain is valid only when every step is verified.
```

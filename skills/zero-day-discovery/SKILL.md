---
name: zero-day-discovery
description: >-
  0-day autonomous discovery engine: variant analysis / patch gap / differential testing / fuzzing / taint reasoning / N-day weaponization / hunter mindset. Use when public vulns not found and need to discover 0day or weaponize N-day.
metadata:
  tags: [penetration-testing, red-team]
---

## 0-Day Autonomous Discovery Engine (mine your own when nothing is found online)

```
Core shift: from "matching known vulnerability databases" → "understand how the code/protocol works, and reason about where it breaks." 0-day is not luck — it is method.
Five main paths:
  1. Variant analysis (highest yield): take one CVE patch → extract the vulnerability pattern → grep the entire codebase for the same pattern in other locations → locations the patch didn't cover = 0-day
  2. Patch gap: read the patch's filter logic; blacklists can almost always be bypassed (missed some encoding / equivalent function / alias) → new CVE
  3. Differential testing: two components disagree on the same input (WAF vs backend / validator vs executor) → smuggling / SSRF bypass
  4. Fuzzing: write harness (wrap the function processing untrusted input) + generate corpus/dictionary + crash triage (reproducible / controllable / exploitable?)
     AFL++ / libFuzzer (coverage-guided to find memory corruption) | boofuzz (protocol) | radamsa (blackbox) | restler (REST API)
  5. Taint reasoning (most powerful with source code): source (parameters / headers / deserialization fields) with no effective sanitizer reaching sink (exec / SQL / template) = 0-day
     CodeQL queries to automatically solve data-flow reachability / Semgrep / Joern
N-day weaponization (advisory published but no public PoC): reverse-engineer exploit from patch diff (bindiff/diaphora for binary comparison;
  vendor regression tests are often the PoC skeleton) → refine in local lab → hit target. Maximizing the vulnerability window is one of a red team's most valuable capabilities.
Hunter mindset (interrogate every piece of code/endpoint/protocol layer by layer; every "yes" is a 0-day candidate):
  Trust boundary: is the input assumed to be trusted? When does that assumption break? | State/timing: can state be changed between two steps (TOCTOU)? Can order be disrupted?
  Parsing/normalization: how many times is it parsed? Is normalize before or after validation? | Boundary extremes: negative numbers / zero / overflow / type confusion / encoding / null byte?
  Implicit capability: what does this feature "incidentally" give you? | Uniqueness: is the ID/token predictable? Is the "secret" truly secret?
Turn every component into an "assumption list" and break each one: file upload assumes "only images / extension is trustworthy / filename has no path / content is only data" → break each = vulnerability; combine = chain.
0-day validation (higher bar than CVE): reproducible (minimal PoC) + root cause clear (which line / which assumption) + impact demonstrable (actual read/write/execute) + false positive ruled out.
```

---
name: source-code-hunting
description: >-
  Source code hunting: .git leak, dangerous-function grep, JS RC4 deobfuscation, semgrep/CodeQL, trufflehog, patch diff, supply chain/CI. Use when hunting source leaks, secrets, JS deobfuscation, or supply-chain issues.
metadata:
  tags: [penetration-testing, red-team]
---

## Source Code Hunting

```
=== Source Code Hunting ===
.git leak: git-dumper → git log -p --all (previously deleted sensitive files) | .svn / .DS_Store / composer.lock
Dangerous function grep: exec/system/eval/unserialize/pickle.loads/render/curl_exec + hardcoded keys (sk-/ghp_/BEGIN RSA)
JS deobfuscation (RC4+base64 string-array pattern): 1) extract string array (var a0G=[...]) 2) find decode function (a0m(idx,key) → RC4 decrypt + base64) 3) find rotation IIFE (target offset) 4) rebuild decoder in Node.js and batch-decode all strings → get plaintext variable names / API paths / config
  UniApp characteristics: app-service.js (business logic, often 800KB+ obfuscated) + zlsioh.dat (encrypted config, decrypted by native .so) + dcloud_uniplugins.json (plugin manifest)
Static analysis: semgrep --config=auto for quick scan / CodeQL: build DB, write queries (taint solving + variant analysis methodology — see `zero-day-discovery`)
Secrets deep mining: trufflehog/gitleaks to scan full git history + Docker image layers + npm/PyPI tarballs + frontend bundles (--only-verified to distinguish live vs dead keys)
Framework patch diff: clone pre/post versions, diff → what was fixed = where the vulnerability is | dependency chain: composer.json / npm audit / pip-audit
Supply chain / CI: dependency confusion (register internal package name on public registry) | GHA command injection ${{github.event.issue.title}} | self-hosted runner takeover | .npmrc / .pypirc credentials
```

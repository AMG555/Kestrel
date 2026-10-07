---
name: kestrel-eino-demo
description: Full-featured example skill package with SKILL.md + optional scripts/, references/, assets/ directories; validates Eino skill and HTTP package-internal paths (authorized security testing and education only).
---

# Kestrel × Eino Full-Featured Skill Demo

This package aligns with [Agent Skills](https://platform.claude.com/docs/en/agents-and-tools/agent-skills/overview): **`SKILL.md` is the manifest + main description** (no separate `SKILL.yaml`). The same directory may contain any subdirectories such as **`scripts/`**, **`references/`**, **`assets/`** (as long as paths are safe and within package depth/file-count limits), read by **`ListPackageFiles` / `resource_path`** and Eino native tools. See `FORMS.md` and `REFERENCE.md` for additional notes.

## Overview

Used for one-shot validation of:

- HTTP `GET /api/skills` listing (`script_count`, `file_count`, `progressive`, etc. are derived/scanned results)
- `GET /api/skills/kestrel-eino-demo?depth=summary|full`
- `section=` corresponds to `##` headings or ASCII heading short IDs in `SKILL.md` (e.g. `## Payload Examples` often maps to `section=payload`)
- Multi-agent ADK **`skill`** tool (and optional native file tools) reading package-relative path resources
- Eino `FilesystemSkillsRetriever` retrieval of package summaries, `##` chunks, and script entries

**Hard requirement**: all testing must have written authorization, confined to the agreed scope and time window.

## Authorized Testing Workflow

1. **Scope confirmation**: domain / IP, interface list, prohibited actions (DoS, data exfiltration, etc.).
2. **Baseline recording**: read-only probing of agreed assets, save timestamps and original request/response summaries.
3. **Categorized testing**: break work into vulnerability types; re-confirm authorization boundary before high-risk operations.
4. **Evidence and report**: each finding includes reproduction steps, impact, remediation recommendation; desensitize sensitive data.
5. **Cleanup**: delete temporary accounts, clean up test data, deliver report.

## Payload Examples

The following are **educational placeholders** — replace with target context in real testing and do not use against unauthorized systems:

- SQLi probe (error-based): `"'` (observe whether database error is leaked)
- XSS reflected (sanitized): `<script>alert(1)</script>` → should be encoded or blocked by CSP in a test environment
- Path traversal (read-only verification): `....//....//etc/passwd` (only in authorized file-read scenarios)

See `scripts/payloads.txt` for the full list.

## references/ and assets/

Used to verify that non-`scripts/` subdirectories are treated equally:

| Path | Purpose |
|------|---------|
| `references/citations.md` | Citations and HTTP `resource_path` test notes |
| `assets/README.txt` | Placeholder resource (can be replaced with real binary for file-read limit testing) |

## Recommended Toolchain

| Phase | Tool examples |
|-------|--------------|
| Proxy and replay | Burp Suite, mitmproxy |
| Scanning and directory brute-force | ffuf, nuclei (lower concurrency to comply with authorization) |
| Vulnerability validation | custom PoC, official CLI (sqlmap etc.) — authorized scope only |
| Recording | Markdown + JSON snippet templates (see `scripts/report-snippet.json`) |

## Checklist and Validation

- [ ] Written authorization and test window saved
- [ ] Files under `scripts/` match body references
- [ ] Web or `GET /api/skills?...` can verify index; in multi-agent sessions use **`skill`** tool to load by package to save tokens
- [ ] Use **`skill`** to pull full text when details are needed, or HTTP `depth=full`, `section=<heading or short id>`
- [ ] Use native file tools or HTTP `resource_path=scripts/check-env.sh` to get raw script content
- [ ] `resource_path=references/citations.md` and `resource_path=assets/README.txt` are readable

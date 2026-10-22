# Kestrel CI Gate & SARIF 2.1.0 Exporter (Strix Integration Plan)

## Problem Statement
How might we enable AppSec and DevSecOps engineers to run Kestrel autonomously inside CI/CD pipelines, producing standardized SARIF 2.1.0 reports with proven exploit PoCs that block vulnerable pull requests in GitHub and GitLab?

## Recommended Direction
Add native SARIF 2.1.0 report generation in `internal/report/sarif.go` and a headless CLI scan mode in `cmd/server/main.go`.

When executed in CI:
```bash
kestrel scan --target ./ --format sarif --out findings.sarif --fail-on high --timeout 10m
```

Kestrel runs targeted reconnaissance/pentest agents using its existing Go multi-agent engine (`internal/multiagent`), executes tools locally via `internal/processguard` (or optionally within a Kali Docker tool sidecar), and streams discovered `database.Vulnerability` records directly into OASIS SARIF 2.1.0 format. The process exits with code `0` (clean) or `2` (vulnerabilities found >= fail threshold) to natively control CI quality gates.

## Architectural Comparison & Synergies (Strix vs Kestrel)

| Capability | Strix (`usestrix/strix`) | Kestrel (`AMG555/Kestrel`) | Integration Plan for Kestrel |
|---|---|---|---|
| **Vulnerability Schema** | Custom JSON + SARIF 2.1.0 (`strix/report/sarif.py`) | `database.Vulnerability` in SQLite | Map Kestrel vulnerability fields directly to SARIF rules, results, and locations |
| **CI/CD Scan Gate** | CLI headless mode (`strix -n -t ./`) with exit codes 0/1/2 | Server daemon HTTP mode only | Add headless CLI entrypoint `kestrel scan` skipping web UI and admin auth setup |
| **Tool Execution** | Hard dependency on Docker Kali container | Native host process containment (`internal/processguard`) | Hybrid model: native `processguard` default; optional Docker container for Kali tools |
| **HTTP Interception** | Local Caido GraphQL API bridge (`strix/tools/proxy`) | Static tool wrappers (`zap.yaml`, `curl`) | (Milestone 2) Add Caido tool bridge for dynamic request tampering |
| **Blind Flaw Proofs** | Integrated `interactsh-client` | Static `dnslog.yaml` | (Milestone 2) In-tree Go out-of-band listener to eliminate SSRF/RCE hallucinations |
| **Remediation** | `apply_patch` generates unified diffs | Manual operator remediation | (Milestone 3) AI-generated unified diff patch workflow |

## Key Assumptions to Validate
- [ ] SARIF output passes official OASIS SARIF validator and displays cleanly in `github/codeql-action/upload-sarif`.
- [ ] Headless scan mode executes completely non-interactively without prompting for admin credentials or initializing browser UI listeners.
- [ ] Process isolation (`processguard`) reliably enforces 10-minute task timeouts and reaps orphan child processes on runner `SIGTERM`.

## MVP Scope (What's In)
1. **SARIF 2.1.0 Exporter (`internal/report/sarif.go`)**:
   - Go structs for OASIS SARIF 2.1.0 specification.
   - Severity level mapping:
     - `critical` -> `error` (security-severity: "9.0")
     - `high` -> `error` (security-severity: "7.5")
     - `medium` -> `warning` (security-severity: "5.0")
     - `low` -> `note` (security-severity: "2.5")
     - `info` -> `note` (security-severity: "1.0")
   - CWE rule catalog mapping for common AppSec classifications.
   - Target URI and physical location formatting with line number fallback.
2. **REST API Endpoint (`internal/handler/vulnerability.go`)**:
   - `GET /api/v1/projects/:id/export/sarif` for UI/API downloads.
3. **Headless CLI Command (`cmd/server/main.go`)**:
   - `--scan`: headless scanner flag.
   - `--target`: directory, URL, or OpenAPI schema.
   - `--format`: `sarif`, `json`, `markdown` (default `sarif`).
   - `--out`: destination file path.
   - `--fail-on`: threshold (`critical`, `high`, `medium`, `low`).
   - Exit codes: `0` (clean / below threshold), `1` (fatal execution error), `2` (vulnerabilities found >= threshold).
4. **CI Workflow Template**:
   - `.github/workflows/kestrel-scan.yml` demonstrating GitHub Action step and SARIF upload.

## Not Doing in MVP (and Why)
- **Interactive Proxying (Caido)**: High setup overhead for CI runner environments; reserve for live manual pentest workflows.
- **Auto-Fix Code Modifying (`apply_patch`)**: Writing code changes directly in repositories requires operator trust; verify scanning accuracy first.
- **Mandatory Docker Container Requirement**: Preserves Kestrel's core value proposition of single-binary on-premise execution with zero container dependencies.

## Open Questions & Answers
- **Remote vs Local Locations**: If target is a live web app or API rather than source repo, SARIF `artifactLocation.uri` uses the endpoint URL (`https://api.example.com/v1/...`) and logical location objects (`fullyQualifiedName`).

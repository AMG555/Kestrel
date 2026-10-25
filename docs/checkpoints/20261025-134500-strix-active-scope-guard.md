---
status: completed
branch: main
timestamp: 2026-10-25T13:45:00+05:30
commits:
  - b6ecd4c: feat(toolguard): add Active Scope Guard for CIDR, wildcard domains, and cloud IMDS protection
---

## Working on: strix-active-scope-guard

### Summary
Milestone 8 completed. Implemented programmatic Active Scope Guard inside ToolGuard. Enforces target authorization boundaries across CIDR blocks, wildcard domain rules, explicit exclusions, and unconditionally forbids access to cloud instance metadata service (IMDS) endpoints.

### Completed Work
- [x] Implemented `internal/toolguard/scope.go`:
  - `ValidateTargetScope(target string, scope ProjectScope) ScopeEvaluationResult`: Evaluates candidate targets against authorized project boundaries.
  - `ExtractHost(target string)`: Robust canonical host and IP extractor handling raw URLs, ports, brackets, and path prefixes.
  - `IsCloudMetadataEndpoint(host string)`: Cloud IMDS protection blocking `169.254.169.254`, `fd00:ec2::254`, `metadata.google.internal`, and the entire `169.254.0.0/16` IPv4 link-local range.
  - `MatchDomainPattern(host, pattern string)`: Wildcard and subdomain matching (`*.acme.corp`, `acme.corp`), preventing attacker domain bypasses (`acme.corp.attacker.com`).
  - `MatchCIDRPattern(host, pattern string)`: IPv4/IPv6 CIDR subnet matching (`10.0.0.0/8`, `192.168.1.0/24`).
- [x] Comprehensive test coverage in `internal/toolguard/scope_test.go`:
  - Verified cloud IMDS blocking even if wildcard `*` is specified.
  - Verified authorized domain matching, subdomain matching, and rejection of out-of-scope/spoofed domains.
  - Verified CIDR matching and exclusion precedence.
  - 100% test pass.

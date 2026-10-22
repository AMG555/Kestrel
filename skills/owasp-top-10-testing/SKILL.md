---
name: owasp-top-10-testing
description: >-
  Systematic testing against OWASP Top 10 (2025 edition): Broken Access Control (with SSRF), Security Misconfiguration, Software Supply Chain Failures, Cryptographic Failures, Injection, Insecure Design, Authentication Failures, Integrity Failures, Logging Failures, Mishandling of Exceptions. Use when performing full-scope AppSec compliance reviews.
metadata:
  tags: [penetration-testing, web-security, owasp, compliance]
---

## OWASP Top 10 (2025 Edition) Assessment Matrix

```
=== A01:2025 Broken Access Control (incl. SSRF) ===
Note: SSRF is officially consolidated under A01 in 2025 edition.
1. Path Traversal: ../../../etc/passwd, %2e%2e%2f, Spring Boot /actuator/../api/
2. IDOR / Horizontal Privilege Escalation: tamper record IDs in request path or body.
3. Vertical Privilege Escalation: access /admin or execute administrative actions with standard user token.
4. SSRF (Server-Side Request Forgery): probe internal services, metadata IP (169.254.169.254), or OOB interactsh callbacks.
5. Missing CORS controls: Origin reflection with Access-Control-Allow-Credentials: true.

=== A02:2025 Security Misconfiguration ===
1. Default credentials across services (admin/admin, guest/guest).
2. Exposed debug/health endpoints (/actuator, /metrics, /swagger, /_profiler, phpinfo.php).
3. Verbose error stacks disclosing framework versions, file paths, database queries.
4. Missing security headers: Content-Security-Policy, Strict-Transport-Security, X-Frame-Options.
5. Permissive directory indexing (Apache/Nginx autoindex).

=== A03:2025 Software Supply Chain Failures ===
1. Outdated third-party components with known CVEs (retire.js, trivy, govulncheck).
2. Vulnerable frontend dependencies (jQuery, Angular, Vue, lodash prototype pollution).
3. Unpinned Docker base images or public script inclusions (<script src="http://untrusted-cdn">).

=== A04:2025 Cryptographic Failures ===
1. Plain HTTP transmission of sensitive data, passwords, or session tokens.
2. Weak TLS configuration (TLS 1.0/1.1, insecure cipher suites).
3. Hardcoded secrets, private keys, API tokens in client-side JS bundles.
4. Broken password hashing (MD5, SHA1, plain SHA256 without salt/bcrypt).

=== A05:2025 Injection ===
1. SQL Injection: UNION SELECT, error-based, time-based blind (pg_sleep, sleep).
2. OS Command Injection: ; $(cmd) `cmd` %0a with space bypass ($IFS).
3. Server-Side Template Injection (SSTI): {{7*7}}, ${7*7}, #{7*7}.
4. Cross-Site Scripting (XSS): Stored, Reflected, DOM-based (<img src=x onerror=...>).
5. LDAP / XPath / NoSQL Injection: {"$gt":""}, {"$ne":null}.

=== A06:2025 Insecure Design ===
1. Lack of business logic rate limiting on payment, refund, or registration endpoints.
2. Insecure password recovery architecture (predictable security questions).
3. Credential stuffing vulnerability (no bot detection, no IP throttling).

=== A07:2025 Authentication Failures ===
1. Permissive credential brute forcing without account lockout or CAPTCHA.
2. Session fixation: session ID does not rotate upon login.
3. Insecure session expiration: tokens remain valid indefinitely after logout.
4. Weak password policy permitting single-character or common dictionary passwords.

=== A08:2025 Software & Data Integrity Failures ===
1. Insecure Deserialization: Python pickle, Java ObjectInputStream, PHP unserialize, Node-serialize.
2. Subresource Integrity (SRI) missing on external CDN scripts.
3. Insecure file upload allowing arbitrary code execution (webshell upload).

=== A09:2025 Security Logging and Monitoring Failures ===
1. Authentication failures and authorization denials not logged.
2. High-value transactions logged without audit trail or immutable timestamping.
3. Log injection: CRLF characters (\r\n) injected into logged parameters.

=== A10:2025 Mishandling of Exceptional Conditions (New in 2025) ===
1. Fail-open behaviors: authorization checks failing into an allowed state when database or auth service is down.
2. Unhandled panics/exceptions causing service denial or leaking internal state.
3. Type confusion or null pointer crashes in parser logic.
```

## Agent Execution Rules
1. Map every confirmed finding to exact OWASP 2025 category code (e.g. `A01:2025`, `A05:2025`).
2. Attach concrete reproduction command and response proof.
3. When saving vulnerability, set `vulnerability_type` to category identifier for automatic SARIF rule indexing.

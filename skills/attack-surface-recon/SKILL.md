---
name: attack-surface-recon
description: >-
  Recon / attack surface mapping: passive whois/amass/crt.sh/FOFA/Shodan, active subfinder/httpx/naabu/katana/nuclei, DNS geolocation/CDN/Nginx catch-all/BT Panel/UniApp fingerprinting. First action at engagement start; write findings to project blackboard. Use when starting recon, asset mapping, fingerprinting, or CDN/DNS bypass discovery.
metadata:
  tags: [penetration-testing, red-team]
---

## Recon / Attack Surface Mapping

```
=== Recon / Attack Surface Mapping (first action at engagement start; 60% of ops time is here — don't run blind into exploitation; write ports/fingerprints to upsert_project_fact immediately) ===
Passive first (no direct target contact — execute all):
  terminal: whois {domain} | grep -iE 'org|name|email|registrant'
  terminal: amass intel -asn {ASN} -d {domain}
  browser_navigate: https://crt.sh/?q=%.{domain} (certificate transparency subdomain discovery)
  terminal: curl -s "https://crt.sh/?q=%.{domain}&output=json" | python3 -c "import sys,json;[print(x['name_value']) for x in json.load(sys.stdin)]" | sort -u
  terminal: dig +short {domain} @114.114.114.114; dig +short {domain} @8.8.8.8 (compare differences = CDN)
  terminal: curl -s "https://web.archive.org/cdx/search/cdx?url=*.{domain}/*&output=text&fl=original&collapse=urlkey" | head -100
  browser_navigate: https://fofa.info/result?qbase64=$(echo -n 'domain="{domain}"' | base64)
  browser_navigate: https://www.shodan.io/search?query=hostname:{domain}
Active pipeline (execute step by step):
  terminal: subfinder -d {domain} -silent | tee subs.txt; amass enum -d {domain} -passive >> subs.txt; sort -u subs.txt -o subs.txt
  terminal: cat subs.txt | dnsx -silent | tee alive_subs.txt
  terminal: cat alive_subs.txt | httpx -silent -title -status-code -tech-detect -cdn | tee httpx_out.txt
  terminal: naabu -l alive_subs.txt -top-ports 1000 -silent | tee ports.txt; nmap -sCV -iL <(head -20 alive_subs.txt) -oN nmap_out.txt
  terminal: katana -list alive_subs.txt -silent -d 3 | tee urls.txt; gau {domain} >> urls.txt
  terminal: cat urls.txt | grep -iE '\.(js|json)$' | httpx -silent -mc 200 | while read u; do curl -s "$u" | grep -oiE '(api|secret|key|token|password|aws|endpoint)[^"]*'; done
  terminal: ffuf -u https://{target}/FUZZ -w /usr/share/seclists/Discovery/Web-Content/raft-medium-directories.txt -mc 200,301,302,403 -fs {catch-all-size}
  terminal: nuclei -l alive_subs.txt -t /root/nuclei-templates/ -severity medium,high,critical -o nuclei_results.txt
  → httpx output identifies framework/version → immediately trigger `component-vuln-intel` search sequence
🚨DNS geolocation restriction bypass: Domestic CDN (987dns/dnspod/Aliyun) often returns 0.0.0.0 to overseas DNS; use 114.114.114.114 to get the real IP. Compare multiple DNS servers: dig @1.1.1.1 vs @114.114.114.114 vs @8.8.8.8 — differences indicate CDN geolocation policy. Real IP is hidden in domestic DNS results.
🚨CDN response filtering bypass (GCCDN/PWS etc.): CDN blocks specific URL patterns (admin paths/actuator) with 502 instead of forwarding backend responses:
  Fingerprinting: Server: PWS/x.x.x.x | Via: 1.1 PS-XXX-XXXX:N (W) | X-Px: ms CS-XXX-XXXXnone(origin) | wildcard *.gccdn.net CNAME
  502 ≠ doesn't exist: 502 = backend response filtered by CDN, 404 = path truly doesn't exist, 403 = CDN routing denied. Use 502/404/403 differences to enumerate live endpoints
  Path traversal confirmation: /api/v1/v/..;/..;/actuator/env → 502 (exists but filtered) vs /api/v1/v/..;/..;/xxxx → 404 (Tomcat native not found)
  CDN node multi-port: scan CDN IP ports 3000-9200 (8000/8080/9090 often proxy same backend but rules may differ, 9200 may be separate nginx)
  Bypass attempt matrix (all failed this session — recorded to prevent repetition): URL encoding / double encoding / case variation / semicolon suffix / TE space smuggling / HTTP pipelining / Accept-Encoding / Range
  Useful info even on 502: POST actuator/env may still be executed by backend (blind property write) → requires refresh trigger (if refresh returns 404, chain is broken)
🚨Nginx Catch-All trap: PHP CMS configured with rewrite for all routes → all paths return 200 with same size = catch-all (not file existence!). Identify: use 3 random paths and compare sizes — identical sizes = catch-all. Test truly live paths with .php extension (nginx `location ~ \.php$` goes straight to FastCGI; non-existent returns real 404).
🚨Pure-FTPd puredb corruption ≠ auth bypass: "Unable to read the indexed puredb file" = virtual user DB corrupted, all usernames (including anonymous) trigger 421 disconnect. PAM system user auth also goes through puredb first → all fail. Not an attack surface — skip.
🚨Same-IP lateral movement: reverse-lookup IP (SecurityTrails/ThreatBook/VirusTotal) to find other domains on the same server → weak sites (independent nginx config returning real 404 = enumerable) → break in → BT Panel unified management = lateral access to all sites.
Fingerprint priority: once httpx/wappalyzer identifies a framework version → immediately pivot to `component-vuln-intel` for threat intel. Map attack surface first, then reason about the most valuable edge — don't dive into the first vulnerability you find.
🚨Wildcard CDN detection: all subdomains (including random non-existent ones) resolve to CDN = wildcard DNS (*.domain → CDN CNAME). Subdomain enumeration is pointless — all IPs are CDN nodes.
  Verify: dig random123456.target.com → if it also has an A record pointing to a known CDN IP = wildcard. Real source IP must be obtained through other channels (historical DNS / cert transparency / other domains on same server / email headers / SSRF)
🚨BT Panel fingerprinting: Cookie name is 32-char hex hash (e.g. 789d6d2de16419a1e8bbfea926c8996e) + 302 redirect to /login + nginx reverse proxy + "entry validation failed" page = BT Panel random security entry path
  Windows version: IIS+nginx coexistence, port 8888 panel, port 888 phpMyAdmin (possible), /index.html may leak site config (reverse proxy target URL / site name)
  Attack: brute-force entry path (8-char random alphanumeric) | historical CVEs (API key unauthorized / phar deserialization) | panel default FTP/MySQL credential reuse
🚨Nginx 404 differential fingerprint: when nginx catch-all rewrite masks real paths, compare response sizes — real nginx 404 (146B) = file exists on disk but location block denies; catch-all (large page) = route unmatched. Bulk probe CMS marker paths (/caches/configs/, /phpcms/modules/, /statics/ etc.) — size differences confirm CMS type. See references/nginx-404-differential-fingerprinting.md
🚨UniApp/DCloud APK quick reversing path: assets/apps/__UNI__*/www/app-service.js = all business logic (JS obfuscated); assets/zlsioh.dat = encrypted config (API domains); lib/arm64-v8a/libirzlemnr.so = decryption library. Deobfuscate JS with RC4+base64 (extract a0G array + a0m decoder); strings in .so: search for 16-char random strings = possible keys; search for `x<d>.<d>.<d>.<d>x` format = hardcoded IPs (usually DCloud HTTPDNS infrastructure, not target API). See references/uniapp-dcloud-apk-reversing.md
```

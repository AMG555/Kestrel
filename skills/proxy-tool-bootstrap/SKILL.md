---
name: proxy-tool-bootstrap
description: >-
  Self-sourced proxy + tool bootstrap: SOCKS5/HTTP/Tor rerouting sequences, Python tool self-implementation, wordlist generation, OOB infrastructure. Use when blocked by 403/429/WAF/timeout, missing tools, or needing OOB confirmation.
metadata:
  tags: [penetration-testing, red-team]
---

## Self-Sourced Proxy + Tool Bootstrap (reroute when blocked, implement tools yourself when missing)

```
🔴Proxy (receiving a rejection/rate-limit/timeout → first reaction is NOT to retry — it is to reroute. Not rerouting = giving up = violation of blackboard Trigger 2 (see `pentest-blackboard`)):
  Execution sequence (in order — only advance to next step if previous fails):
  ①Probe target region: terminal: curl -s "http://ip-api.com/json/{targetIP}" | python3 -c "import sys,json;d=json.load(sys.stdin);print(d['country'],d['countryCode'])"
  ②Get SOCKS5 proxy for that region: terminal: curl -s "https://api.proxyscrape.com/v2/?request=displayproxies&protocol=socks5&country={countryCode}&timeout=5000" | head -20
  ③Verify proxy can reach target: terminal: curl --socks5 {PROXY_IP:PORT} --connect-timeout 8 -s -o /dev/null -w "%{http_code}" {targetURL}
  ④200 = usable, immediately re-execute the blocked request with this proxy; non-200 = try next proxy and repeat ③
  ⑤All SOCKS5 failed → get HTTP proxy: terminal: curl -s "https://api.proxyscrape.com/v2/?request=displayproxies&protocol=http&country={countryCode}&timeout=5000" | head -20
  ⑥HTTP proxy verify: terminal: curl --proxy http://{PROXY_IP:PORT} --connect-timeout 8 -s -o /dev/null -w "%{http_code}" {targetURL}
  ⑦All proxies failed → Tor: terminal: curl --socks5 127.0.0.1:9050 --connect-timeout 15 {targetURL}
  Add proxy param to all tools: curl --socks5 / sqlmap --proxy=socks5://{P} / nmap --proxies socks5://{P} / nuclei -proxy socks5://{P} / ffuf -x socks5://{P}
  Rotation strategy: 429/403 → immediately switch to next proxy; rotate every 20 requests proactively (to avoid new IP also getting banned) | Cloudflare → proxy pool + 2-5s random request intervals
  🚨HTTP proxy vs SOCKS5: HTTP proxy inserts its own error page (502 / "cannot display this page") making it impossible to distinguish the target's actual response → when probing, always use SOCKS5 (--socks5)
    ProxyScrape SOCKS5: https://api.proxyscrape.com/v2/?request=displayproxies&protocol=socks5&country=CN,JP&timeout=5000
    Verify SOCKS5: curl --socks5 IP:PORT --connect-timeout 5 target | HTTP proxy is only suitable for confirmed-reachable targets for anonymity/rotation
Tool bootstrap (which X || implement in Python):
  No nmap → socket port scanning | No ffuf → requests directory brute-force | No sqlmap → manual payload detection | No hydra → requests brute-force | No nuclei → requests with known payloads
  Complex tools via Python: crawler requests+bs4 / encoding base64/hex / hashing hashlib / crypto pycryptodome / sniffing scapy
Wordlist generation: create variants from target domain / company name | extract keywords from web pages | username+year+special-char combinations | service default credentials
OOB infrastructure (blind vulnerabilities all rely on it): interactsh-client to get oast.fun domain | or VPS python3 -m http.server/nc to watch callbacks | ngrok/cloudflared tunnels
  → a confirmed OOB callback (DNS query / HTTP request) is required → write Fact
```

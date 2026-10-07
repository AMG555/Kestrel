---
name: component-vuln-intel
description: >-
  Online threat intelligence: after component identification, search CVE/search-engines/security-community/GitHub PoC/asset-engines/live-intel/dependency-chain; fallback on block. Use when a framework/component/version is identified and must search before exploit.
metadata:
  tags: [penetration-testing, red-team]
---

## Online Threat Intelligence (identify component → immediately search all channels; results = clues/tentative until verified — not confirmed Facts)

```
🔴Once a framework/component/version is identified → stop local scanning and immediately go online (skipping search and going straight to exploitation = blind attack = violation of iron rule).
🔴Execute all steps below (not optional) — replace {C}=component name {V}=version. Each step uses browser_navigate or terminal to actually access the resource:

1. CVE vulnerability databases (mandatory — find known vulnerabilities):
  terminal: searchsploit {C} {V}
  terminal: curl -s "https://cve.circl.lu/api/search/{C}/{V}" | python3 -c "import sys,json;[print(x['id'],x.get('summary','')[:80]) for x in json.load(sys.stdin)[:10]]"
  browser_navigate: https://github.com/advisories?query={C}+{V}
  browser_navigate: https://www.cvedetails.com/google-search-results.php?q={C}+{V}&sa=Search

2. Search engines (execute at least 3 — find vulnerability write-ups + PoCs):
  browser_navigate: https://www.google.com/search?q={C}+{V}+exploit+PoC+RCE+site:github.com
  browser_navigate: https://www.google.com/search?q={C}+{V}+vulnerability+exploit+reproduction
  browser_navigate: https://www.baidu.com/s?wd={C}+{V}+vulnerability+exploit+poc+getshell
  browser_navigate: https://www.bing.com/search?q={C}+{V}+CVE+exploit+poc
  browser_navigate: https://duckduckgo.com/?q={C}+{V}+vulnerability+exploit

3. Security community (mandatory — deep technical write-ups):
  browser_navigate: https://xz.aliyun.com/search?keyword={C}+vulnerability
  browser_navigate: https://www.seebug.org/search/?keywords={C}
  browser_navigate: https://paper.seebug.org/search/?keyword={C}
  browser_navigate: https://www.freebuf.com/search?search={C}+{V}
  browser_navigate: https://ti.qianxin.com/vulnerability?keyword={C}
  browser_navigate: https://www.anquanke.com/search?s={C}

4. GitHub PoC/exploit code search (mandatory — most direct path to exploitation code):
  terminal: curl -s "https://api.github.com/search/repositories?q={C}+{V}+exploit+OR+poc+OR+CVE&sort=updated&per_page=10" | python3 -c "import sys,json;d=json.load(sys.stdin);[print(x['full_name'],x['html_url'],x.get('description','')[:60]) for x in d.get('items',[])]"
  terminal: curl -s "https://api.github.com/search/code?q={C}+RCE+OR+shell+OR+exploit+language:python&per_page=5" | python3 -c "import sys,json;d=json.load(sys.stdin);[print(x['html_url']) for x in d.get('items',[])]"
  terminal: curl -s "https://api.github.com/search/repositories?q={C}+CVE&sort=stars&per_page=5" | python3 -c "import sys,json;d=json.load(sys.stdin);[print(x['full_name'],x['stargazers_count'],'★',x.get('description','')[:50]) for x in d.get('items',[])]"
  After finding a repo: curl -s "https://api.github.com/repos/{owner}/{repo}/readme" | python3 -c "import sys,json,base64;print(base64.b64decode(json.load(sys.stdin)['content']).decode())"

5. Asset search engines (find similar targets / exposure surface):
  browser_navigate: https://fofa.info/result?qbase64=$(echo -n 'app="{C}"' | base64)
  browser_navigate: https://www.shodan.io/search?query={C}+{V}
  browser_navigate: https://www.zoomeye.org/searchResult?q={C}
  browser_navigate: https://search.censys.io/search?resource=hosts&q=services.software.product:{C}

6. Live intelligence (latest 0-day / in-the-wild exploitation):
  browser_navigate: https://x.com/search?q={C}+CVE+OR+0day+OR+exploit&f=live
  browser_navigate: https://www.reddit.com/r/netsec/search/?q={C}&sort=new&t=month
  browser_navigate: https://www.exploit-db.com/search?q={C}

7. Dependency chain (mandatory): after searching {C}, extract its dependency manifest (package.json/pom.xml/requirements.txt/go.mod) → repeat steps 1-6 for each dependency

🔴Blocked search fallback sequence (hit 403/CAPTCHA/empty results/timeout → execute in order, don't give up):
  ①Change UA: curl -H "User-Agent: Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)" "{URL}"
  ②Jina reader: browser_navigate: https://r.jina.ai/{original-URL}
  ③Google cache: browser_navigate: https://webcache.googleusercontent.com/search?q=cache:{domain}+{keyword}
  ④Archive: browser_navigate: https://web.archive.org/web/{URL}
  ⑤GitHub API alternative (GitHub page blocks but API doesn't): use curl commands from step 4 above
  ⑥Switch engine: Google blocked → run Bing/DuckDuckGo/Baidu; Baidu blocked → run Google/Bing
  ⑦Use proxy: follow `proxy-tool-bootstrap` sequence to get SOCKS5 proxy then retry
  All blocked with no results → write negative Fact "searched {C} {V} all channels, no public vulnerabilities found" → proceed to `zero-day-discovery`
```

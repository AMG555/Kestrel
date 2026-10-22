#!/usr/bin/env python3
"""
Caido HTTP Traffic Proxy Tool Bridge for Kestrel.
Enables AI security agents to query intercepted proxy traffic, inspect raw HTTP transactions,
and replay mutated payloads through Caido's auditing proxy.
"""

import sys
import os
import json
import urllib.parse
from typing import Optional, Dict, Any, List

try:
    import requests
except ImportError:
    print(json.dumps({
        "status": "error",
        "message": "Missing 'requests' library. Install it with: pip install requests"
    }, indent=2))
    sys.exit(1)

DEFAULT_CAIDO_URL = os.environ.get("CAIDO_URL", "http://127.0.0.1:8080")
DEFAULT_API_KEY = os.environ.get("CAIDO_API_KEY", "")

def check_health(caido_url: str = DEFAULT_CAIDO_URL, api_key: str = DEFAULT_API_KEY) -> Dict[str, Any]:
    url = caido_url.rstrip("/")
    headers = {"Content-Type": "application/json"}
    if api_key:
        headers["Authorization"] = f"Bearer {api_key}"
        
    # Caido exposes a GraphQL endpoint at /graphql
    gql_url = f"{url}/graphql"
    query = {"query": "{ viewer { id } }"}
    
    try:
        resp = requests.post(gql_url, json=query, headers=headers, timeout=4)
        if resp.status_code == 200:
            return {
                "status": "online",
                "caido_url": url,
                "message": "Caido proxy backend is online and responding to GraphQL queries.",
                "graphql_endpoint": gql_url,
                "details": resp.json()
            }
        elif resp.status_code == 401 or resp.status_code == 403:
            return {
                "status": "auth_required",
                "caido_url": url,
                "message": "Caido proxy requires authentication token. Provide api_key or set CAIDO_API_KEY.",
                "status_code": resp.status_code
            }
        else:
            return {
                "status": "warning",
                "caido_url": url,
                "message": f"Caido returned HTTP {resp.status_code}: {resp.text[:200]}"
            }
    except requests.exceptions.ConnectionError:
        return {
            "status": "offline",
            "caido_url": url,
            "message": f"Cannot connect to Caido at {url}. Service is offline or not started.",
            "suggestion": "Start Caido Desktop or Caido CLI (e.g. 'caido --listen 127.0.0.1:8080') to enable proxy traffic auditing."
        }
    except Exception as e:
        return {
            "status": "error",
            "caido_url": url,
            "message": f"Caido health check failed: {str(e)}"
        }

def query_traffic(caido_url: str = DEFAULT_CAIDO_URL, host_filter: str = "", limit: int = 25, api_key: str = DEFAULT_API_KEY) -> Dict[str, Any]:
    url = caido_url.rstrip("/")
    gql_url = f"{url}/graphql"
    headers = {"Content-Type": "application/json"}
    if api_key:
        headers["Authorization"] = f"Bearer {api_key}"
        
    gql_query = """
    query GetRequests($first: Int) {
      requests(first: $first) {
        edges {
          node {
            id
            host
            port
            isTls
            method
            path
            query
            response {
              statusCode
              roundtripTime
            }
          }
        }
      }
    }
    """
    
    try:
        resp = requests.post(gql_url, json={"query": gql_query, "variables": {"first": limit}}, headers=headers, timeout=8)
        if resp.status_code != 200:
            return {
                "status": "error",
                "message": f"Caido GraphQL request failed with HTTP {resp.status_code}: {resp.text[:200]}"
            }
            
        data = resp.json()
        if "errors" in data:
            return {
                "status": "error",
                "message": "Caido GraphQL error",
                "errors": data["errors"]
            }
            
        edges = data.get("data", {}).get("requests", {}).get("edges", [])
        requests_list = []
        for edge in edges:
            node = edge.get("node", {})
            if host_filter and host_filter.lower() not in (node.get("host") or "").lower():
                continue
            requests_list.append({
                "id": node.get("id"),
                "method": node.get("method"),
                "host": node.get("host"),
                "port": node.get("port"),
                "path": node.get("path"),
                "query": node.get("query"),
                "status_code": (node.get("response") or {}).get("statusCode"),
                "roundtrip_ms": (node.get("response") or {}).get("roundtripTime")
            })
            
        return {
            "status": "success",
            "total": len(requests_list),
            "requests": requests_list,
            "filter": {"host": host_filter, "limit": limit}
        }
    except requests.exceptions.ConnectionError:
        return {
            "status": "offline",
            "message": f"Caido proxy not reachable at {url}.",
            "suggestion": "Start Caido or ensure correct port."
        }
    except Exception as e:
        return {
            "status": "error",
            "message": f"Query traffic failed: {str(e)}"
        }

def get_request(request_id: str, caido_url: str = DEFAULT_CAIDO_URL, api_key: str = DEFAULT_API_KEY) -> Dict[str, Any]:
    url = caido_url.rstrip("/")
    gql_url = f"{url}/graphql"
    headers = {"Content-Type": "application/json"}
    if api_key:
        headers["Authorization"] = f"Bearer {api_key}"
        
    gql_query = """
    query GetRequest($id: ID!) {
      request(id: $id) {
        id
        host
        port
        isTls
        method
        path
        query
        raw
        response {
          statusCode
          raw
          roundtripTime
        }
      }
    }
    """
    
    try:
        resp = requests.post(gql_url, json={"query": gql_query, "variables": {"id": request_id}}, headers=headers, timeout=8)
        if resp.status_code != 200:
            return {"status": "error", "message": f"HTTP {resp.status_code}: {resp.text[:200]}"}
            
        data = resp.json()
        req_node = data.get("data", {}).get("request")
        if not req_node:
            return {"status": "not_found", "message": f"Request ID '{request_id}' not found in Caido."}
            
        return {
            "status": "success",
            "request": req_node
        }
    except Exception as e:
        return {"status": "error", "message": f"Failed to retrieve request {request_id}: {str(e)}"}

def replay_request(
    target_url: str,
    method: str = "GET",
    payload: str = "",
    headers_json: str = "",
    proxy_url: str = DEFAULT_CAIDO_URL
) -> Dict[str, Any]:
    headers = {}
    if headers_json:
        try:
            parsed = json.loads(headers_json)
            if isinstance(parsed, dict):
                headers = parsed
        except Exception:
            pass
            
    proxies = {}
    if proxy_url:
        proxies = {
            "http": proxy_url,
            "https": proxy_url
        }
        
    try:
        method_upper = method.upper().strip()
        resp = requests.request(
            method=method_upper,
            url=target_url,
            data=payload if payload else None,
            headers=headers,
            proxies=proxies,
            verify=False,
            timeout=15
        )
        
        # Format response summary
        resp_headers = dict(resp.headers)
        body_text = resp.text
        if len(body_text) > 4000:
            body_text = body_text[:4000] + "\n...[truncated for display]..."
            
        return {
            "status": "success",
            "target": target_url,
            "method": method_upper,
            "proxy_used": proxy_url,
            "status_code": resp.status_code,
            "reason": resp.reason,
            "response_time_ms": int(resp.elapsed.total_seconds() * 1000),
            "response_headers": resp_headers,
            "response_body": body_text,
            "note": "Request proxied through Caido; logged in Caido timeline for manual inspection."
        }
    except requests.exceptions.ProxyError as pe:
        # If proxy connection failed, attempt direct replay with warning
        return {
            "status": "proxy_error",
            "message": f"Failed to connect through Caido proxy ({proxy_url}): {str(pe)}",
            "suggestion": "Ensure Caido proxy is running and configured to accept upstream traffic."
        }
    except requests.exceptions.RequestException as e:
        return {
            "status": "error",
            "message": f"Replay request failed: {str(e)}"
        }

def parse_args():
    args = {
        "operation": "health",
        "target": "",
        "method": "GET",
        "payload": "",
        "headers": "",
        "caido_url": DEFAULT_CAIDO_URL,
        "api_key": DEFAULT_API_KEY
    }
    
    if len(sys.argv) <= 1:
        return args
        
    first_arg = sys.argv[1].strip()
    if first_arg.startswith("{") and first_arg.endswith("}"):
        try:
            data = json.loads(first_arg)
            if isinstance(data, dict):
                args.update(data)
                return args
        except Exception:
            pass

    idx = 1
    positional = []
    while idx < len(sys.argv):
        arg = sys.argv[idx]
        if arg in ("--operation", "-o") and idx + 1 < len(sys.argv):
            args["operation"] = sys.argv[idx + 1]
            idx += 2
        elif arg in ("--target", "-t", "--url") and idx + 1 < len(sys.argv):
            args["target"] = sys.argv[idx + 1]
            idx += 2
        elif arg in ("--method", "-m") and idx + 1 < len(sys.argv):
            args["method"] = sys.argv[idx + 1]
            idx += 2
        elif arg in ("--payload", "-p", "--data") and idx + 1 < len(sys.argv):
            args["payload"] = sys.argv[idx + 1]
            idx += 2
        elif arg in ("--headers", "-H") and idx + 1 < len(sys.argv):
            args["headers"] = sys.argv[idx + 1]
            idx += 2
        elif arg in ("--caido-url", "-c") and idx + 1 < len(sys.argv):
            args["caido_url"] = sys.argv[idx + 1]
            idx += 2
        elif arg in ("--api-key", "-k") and idx + 1 < len(sys.argv):
            args["api_key"] = sys.argv[idx + 1]
            idx += 2
        else:
            positional.append(arg)
            idx += 1

    if len(positional) > 0:
        args["operation"] = positional[0]
    if len(positional) > 1:
        args["target"] = positional[1]
    if len(positional) > 2:
        args["method"] = positional[2]
    if len(positional) > 3:
        args["payload"] = positional[3]
    if len(positional) > 4:
        args["headers"] = positional[4]
    if len(positional) > 5:
        args["caido_url"] = positional[5]
    if len(positional) > 6:
        args["api_key"] = positional[6]

    return args

def main():
    import urllib3
    urllib3.disable_warnings(urllib3.exceptions.InsecureRequestWarning)
    
    config = parse_args()
    op = (config.get("operation") or "health").lower().replace("-", "_")
    caido_url = config.get("caido_url") or DEFAULT_CAIDO_URL
    api_key = config.get("api_key") or DEFAULT_API_KEY
    
    if op in ("health", "status", "ping"):
        res = check_health(caido_url=caido_url, api_key=api_key)
    elif op in ("query_traffic", "list_requests", "traffic"):
        res = query_traffic(
            caido_url=caido_url,
            host_filter=config.get("target") or "",
            limit=25,
            api_key=api_key
        )
    elif op in ("get_request", "inspect"):
        if not config.get("target"):
            res = {"status": "error", "message": "Target parameter (Request ID) is required for get_request operation."}
        else:
            res = get_request(
                request_id=config["target"],
                caido_url=caido_url,
                api_key=api_key
            )
    elif op in ("replay", "send", "mutate"):
        if not config.get("target"):
            res = {"status": "error", "message": "Target parameter (URL) is required for replay operation."}
        else:
            res = replay_request(
                target_url=config["target"],
                method=config.get("method") or "GET",
                payload=config.get("payload") or "",
                headers_json=config.get("headers") or "",
                proxy_url=caido_url
            )
    else:
        res = {
            "status": "error",
            "message": f"Unsupported operation '{op}'. Supported operations: health, query_traffic, get_request, replay."
        }
        
    print(json.dumps(res, indent=2, ensure_ascii=False))
    if res.get("status") in ("error", "proxy_error"):
        sys.exit(1)

if __name__ == "__main__":
    main()

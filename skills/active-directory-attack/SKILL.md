---
name: active-directory-attack
description: >-
  Internal domain attacks: BloodHound, Kerberoast, ADCS ESC1/ESC8, NTLM Relay, Coerce, DACL, DCSync, Zerologon/NoPac/PrintNightmare, mitm6, LLMNR, Linux intranet. Use when attacking Active Directory, ADCS, NTLM relay, or internal domain.
metadata:
  tags: [penetration-testing, red-team]
---

## Internal Domain Attacks

```
=== Internal Domain (2023+ real-world primary battleground) ===
Recon: BloodHound (SharpHound collection → attack paths) | Kerberos: GetNPUsers (AS-REP) / GetUserSPNs (Kerberoast)
🚨ADCS (Certipy one-stop): certipy find -vulnerable | ESC1 specify SAN to request domain-admin cert | ESC8 relay to CA to get DC cert
🚨NTLM Relay (more important than PtH; PtH often blocked by EDR): ntlmrelayx -t ldap--escalate-user / -t http CA --adcs (ESC8) / RBCD
🚨Forced Authentication Coerce: PetitPotam (MS-EFSRPC) / coercer full-protocol spray / printerbug → feed to relay
🚨DACL abuse: WriteDACL → grant self DCSync rights | Shadow credentials certipy shadow (GenericWrite is enough, no password change, no traces)
DCSync: secretsdump -just-dc → krbtgt hash → Golden Ticket
🚨One-shot domain CVEs (test first; if hit, instant domain admin): Zerologon (CVE-2020-1472, zero out DC machine account password → DCSync) | NoPac (CVE-2021-42278/42287, rename machine account to request DC TGT) | PrintNightmare (CVE-2021-34527, print spooler RCE / load malicious driver) | EternalBlue (MS17-010, old SMBv1 direct RCE)
🚨IPv6/mitm6 (must test on dual-stack intranets — default on most modern networks): mitm6 hijacks DHCPv6+DNS → WPAD → ntlmrelayx to LDAP/ADCS (stealthier than LLMNR, preferred on modern intranets)
LLMNR/NBT-NS poisoning: responder captures NetNTLMv2 → hashcat crack / relay
Linux intranet: Redis unauthorized access (CONFIG SET dir write SSH key) | NFS showmount | Docker 2375
```

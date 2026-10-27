---
name: crypto-tls-audit
description: >-
  Cryptographic implementation and TLS transport security auditing: CBC/PKCS7 padding oracle detection and automated decryption/forgery, ECB block analysis and cut-and-paste manipulation, SHA-1/SHA-256 hash length extension attacks with HashPump, AES-GCM nonce reuse and GHASH authentication key recovery, weak PRNG seed recovery (time-based seeds, Mersenne Twister MT19937 state reconstruction), timing side-channel verification, and TLS/SSL protocol downgrade vulnerabilities (POODLE, FREAK, DROWN, BEAST, sweet32, Heartbleed), weak cipher suite audit, and certificate pinning and chain validation checks. Use when auditing cryptography, TLS/SSL configuration, token signing, or encryption implementations.
metadata:
  tags: [cryptography, tls, appsec, encryption-audit]
---

## Cryptographic Implementation & TLS Auditing Playbook

```
=== Phase 1: Symmetric Cipher & Implementation Flaw Auditing ===
1. CBC Mode & PKCS#7 Padding Oracle:
   Identification:
     Modify last byte of ciphertext IV or block.
     Vulnerable signal: server returns distinct response for invalid padding (e.g. 500 "Padding error", 200 vs 403, or timing difference) compared to valid padding with invalid plaintext.
   Automated Decryption & Block Forgery:
     padbuster <URL> <EncryptedSample> <BlockSize> -encoding 0 -cookies "auth=..." -error "Padding error"
     Decrypt arbitrary blocks without knowing secret AES/DES key.
     Forge valid ciphertext for arbitrary plaintext (e.g. inject {"user":"admin","role":"superuser"}).
2. ECB Mode Block Swapping & Byte-at-a-Time Decryption:
   Identification:
     ECB encrypts identical 16-byte plaintext blocks into identical 16-byte ciphertext blocks.
     Send repeated characters: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" -> look for repeating 16-byte hex chunks in ciphertext.
   Exploitation:
     Block rearranging: reorder encrypted user profile chunks to elevate privileges (cut-and-paste attack).
     Byte-at-a-time ECB decryption: control prepended input, vary input length from 15 down to 0 bytes, brute-force unknown byte by matching ciphertext blocks against 256 possible byte values.
3. Hash Length Extension Attacks (MACs without HMAC):
   Vulnerable pattern: server verifies integrity using hash(secret || data) where hash is MD5, SHA-1, SHA-256, or SHA-512.
   Exploitation:
     Knowing len(secret), original data, and signature, attacker appends &admin=true and computes valid signature without knowing secret:
     hashpump -k <secret_length> -s <original_signature> -d "<original_data>" -a "<appended_data>"
   Remediation: migrate from raw hash(secret || message) to HMAC (RFC 2104) or AEAD ciphers.
4. AES-GCM Nonce Reuse:
   Catastrophic flaw: reusing same (Key, Nonce/IV) pair across two distinct messages under AES-GCM.
   Impact: allows recovery of the GHASH authentication hash key (H), enabling arbitrary message forgery and tampering.

=== Phase 2: Randomness, Key Generation & Side-Channel Verification ===
1. Weak PRNG & Predictable Seeds:
   Time-seeded PRNGs: srand(time(NULL)) or System.currentTimeMillis() allows reconstructing pseudo-random sequence across narrow time window.
   Mersenne Twister (MT19937): Python `random`, PHP `rand()`:
     Collecting 624 32-bit outputs allows fully recovering internal state and predicting all future and past random numbers (password reset tokens, session tokens, shuffle orders).
     Fix: enforce cryptographically secure PRNG (/dev/urandom, crypto/rand, secrets module).
2. Timing Side-Channel Analysis:
   Vulnerable pattern: byte-by-byte string comparison using `==` or `strcmp()` instead of constant-time comparison (`crypto/subtle.ConstantTimeCompare`).
   Verification:
     Send candidate tokens varying first byte; measure high-resolution server response time.
     A consistently longer execution time indicates correct prefix match before early return.

=== Phase 3: TLS/SSL Transport Security & Certificate Auditing ===
1. Protocol Downgrade & Legacy Ciphers:
   Scan endpoints for deprecated protocols (SSLv2, SSLv3, TLS 1.0, TLS 1.1):
     tlsx -u target.com -ex -sm
     testssl.sh --protocols --ciphers target.com:443
   Historical exploit checks:
     POODLE (CVE-2014-3566): SSLv3 CBC padding flaw.
     DROWN (CVE-2016-0800): SSLv2 export cipher cross-protocol attack decrypting TLS.
     FREAK (CVE-2015-0204) & Logjam: export-grade RSA/DH key exchange forced downgrade.
     BEAST (CVE-2011-3389): TLS 1.0 predictable IV.
     Sweet32 (CVE-2016-2183): 3DES / Blowfish 64-bit block size collision.
     Heartbleed (CVE-2014-0160): OpenSSL TLS heartbeat memory leak.
2. Certificate Validation & Pinning Auditing:
   Audit checks:
     Hostname mismatch: CN/SAN does not match domain being accessed.
     Self-signed or expired certificate in production trust chain.
     Revocation checking: missing OCSP stapling or CRL verification.
     mTLS client certificate security: verify client certificates cannot be intercepted, replayed, or accepted with invalid intermediate CAs.
```

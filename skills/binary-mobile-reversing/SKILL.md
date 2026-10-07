---
name: binary-mobile-reversing
description: >-
  APK/EXE/binary reversing: UniApp/DCloud/Flutter reversing, certificate pinning bypass, exported components, memory-corruption exploit chains, IoT firmware. Use when reversing APK/EXE, UniApp/Flutter, native .so, or memory-corruption exploits.
metadata:
  tags: [penetration-testing, red-team]
---

## APK / EXE / Binary Reversing

```
=== APK / EXE / Binary ===
APK: apktool d / jadx | Manifest: check exported components / deeplink / debuggable | rg for hardcoded keys + API addresses | .so strings/Ghidra | frida/objection dynamic analysis
🚨UniApp/DCloud APK reversing (H5 hybrid app — all business logic is in JS, not DEX):
  Identify: __UNI__XXXXXXX + assets/apps/ + uni-jsframework.js (NOT Flutter, no libapp.so) | manifest.json contains "uni-app" field
  Core files: assets/apps/__UNI__XXX/www/app-service.js (800KB+ obfuscated JS = all business logic) | jadx can only see the shell (native plugin registration), real logic is in JS
  JS deobfuscation (RC4 + string-array rotation): extract a0G() large array (10000+ elements) + a0m(idx,key) decode function (base64 → RC4) + rotation IIFE → reconstruct executable JS → Node.js batch decode all strings → get plaintext JSON dictionary (offset → plaintext)
    🔴Trap: rotation IIFE may end with a comma (not a semicolon! it is part of a larger expression) → Node.js reports "Unexpected token" → extract according to actual terminator
  Encrypted config zlsioh.dat/dcloud3.dat: header (96B offset table: off40=block1 decompressed/44=compressed/60=block2 offset/64=block2 size/80=block3 offset) + block1 (zlib → DEX) + block2 (encrypted, contains API domain list ← key, needs .so to decrypt) + block3 (zlib → AndroidX class mapping)
    zlib magic 78DA = unencrypted, decompress directly | no magic = encrypted block (key is in .so .rodata / assembly immediate values)
  .so string deobfuscation: extract even-index characters → reverse (e.g. "mAojcl.dubdFiHaebP.nwywfwb" → "www.baidu.com") | strings -n8 lib*.so to find long dot-separated strings
  ⚠️Traps: IP strings with x-separator in .so (e.g. "x111.230.69.120x118.126.105.164") = DCloud HTTPDNS nodes (NOT API backend! 140.205.11.x = Aliyun DNS, 111.230/118.126/106.52/42.193 = Tencent Cloud) | resources.arsc errors are intentional anti-decompile protection (does not affect jadx) | fake PNG assets (1-8px) = integrity verification, not data | fcapp.run = APK download proxy only
  Key API variables: $apiHost/$opHost (set dynamically at runtime, not hardcoded) / urlList / checkAvailableDomainList (HEAD domain/favicon.ico checks connectivity; 200-399 = available)
  High-value endpoints: /app_init /user/gustRegister (guest registration with no auth) /user/getOssSts (OSS temp credentials!) /uploadFile2
  Config: manifest.json (appId / version / nativePlugins → wrs-httpserver = embedded HTTP service!) | supplierconfig.json (vivo/xiaomi/huawei/oppo appid) | dcloud_uniplugins.json (plugin manifest)
  ChengZi SDK: init3 endpoint returns XOR single-byte (key=0x96, see chengzi_decrypt.py) encrypted JSON → decrypt to get real APK download URL (fu field) + channel code
  Domain fallback: when $apiHost cannot be obtained statically → Android emulator (Waydroid/AVD) + tcpdump/mitmproxy to capture runtime DNS
  See: references/uniapp-apk-reverse-engineering.md, references/uniapp-apk-reversing.md, scripts/js_rc4_deobfuscate.js, scripts/chengzi_decrypt.py
🚨Flutter APK quick reversing (no unpacking needed): Protection (MogoSec/Bangbang/360) protects the DEX layer, but Flutter's lib/arm64-v8a/libapp.so (Dart AOT) is usually unencrypted
  strings -n8 libapp.so|grep 'https\?://' → all hardcoded URLs / API domains / S3 addresses / CDN
  strings -n8 libapp.so|grep -E '^/' → API path matrix (/Member/Login, /Web/VideoList, /BBS/GetSTSToken, etc.)
  strings -n8 libapp.so|grep -iE 'key|secret|token|aws|bucket' → credentials / key leaks
  assets/config_*.xml + assets/data_*.dat → encrypted config (may contain domains / API addresses) | .DS_Store → macOS developer info leak | assets/xinstall* → channel tracking SDK
🚨Native .so string deobfuscation (universal patterns): obfuscated strings in .rodata section
  Common patterns: extract even-index characters + reverse (e.g. "mAojcl.dubdFiHaebP..." → take even-index → reverse = plaintext domain) | XOR constant | RC4 + base64
  Identify: strings -n8 lib*.so to find 16-byte equal-length strings (AES key/IV) / dot-separated strings (domains) / x-separated IP strings (HTTPDNS) | nm --dynamic to find obfuscated export symbols (16-char random mixed-case)
  Verify: compare known strings ("classes.dex"/"io.dcloud.application") against obfuscated versions → reverse-engineer algorithm → batch decode
🚨Mobile universal (standard apps that are not UniApp/Flutter):
  Certificate pinning bypass (capture HTTPS): objection android sslpinning disable | frida universal-unpinning | modify smali to delete pinning code and repackage
  Exported component privilege escalation: Manifest exported=true Activity/Service/Provider/Receiver → drozer/adb am start cross-app invocation | ContentProvider SQLi / path traversal
  Deep link hijacking: scheme:// not validated → WebView loads arbitrary URL (XSS / file read) | parameters go to intent → component hijacking
  Insecure storage: /data/data/pkg/ (shared_prefs plaintext token / unencrypted DB / logcat leaks) | world-readable sdcard
  WebView: addJavascriptInterface (<4.2 RCE) / file:// local read / setAllowFileAccess | static analysis: MobSF one-stop | iOS: unpack (frida-ios-dump) + Ghidra for Mach-O + objection dynamic
EXE/PE: file/strings | Ghidra/IDA decompile to find hardcoded / crypto / network logic | x64dbg dynamic debug | Vulns: overflow / format string / UAF / DLL hijacking
🚨Memory corruption exploit chain (crash → RCE): checksec to examine protections → identify primitive (stack overflow / UAF / format string = arbitrary read/write)
  → info leak to bypass ASLR → ROP chain (ROPgadget/pwntools ret2libc) → heap exploitation (tcache poison / __free_hook hijack)
  → arbitrary write to overwrite GOT / hook / vtable / exit_funcs → control flow hijack | IoT firmware: binwalk -Me extract + qemu-user debug (weak protection, direct stack overflow is common)
```

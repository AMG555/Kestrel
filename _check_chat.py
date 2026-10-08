#!/usr/bin/env python3
import re
with open("web/static/js/chat.js", "r", encoding="utf-8") as f:
    lines = f.readlines()
remaining = [(i+1, line.rstrip()) for i, line in enumerate(lines) if re.search(r"[\u4e00-\u9fff]", line)]
print(f"Total lines: {len(remaining)}")
strings = [(n,l) for n,l in remaining if not l.strip().startswith("//") and not l.strip().startswith("*") and not l.strip().startswith("/*")]
comments = [(n,l) for n,l in remaining if l.strip().startswith("//") or l.strip().startswith("*") or l.strip().startswith("/*")]
print(f"String lines: {len(strings)}, Comment lines: {len(comments)}")
print("\nSample strings:")
for n,l in strings[:15]:
    print(f"  L{n}: {l.strip()[:120]}")
print("\nSample comments:")
for n,l in comments[:15]:
    print(f"  L{n}: {l.strip()[:120]}")

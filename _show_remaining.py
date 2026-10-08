#!/usr/bin/env python3
"""Show actual remaining Chinese lines in chat.js and monitor.js."""
import re

for fname in ["web/static/js/chat.js", "web/static/js/monitor.js", "web/static/js/dashboard.js"]:
    with open(fname, "r", encoding="utf-8") as f:
        lines = f.readlines()
    remaining = [(i+1, line.rstrip()) for i, line in enumerate(lines) if re.search(r"[\u4e00-\u9fff]", line)]
    print(f"\n=== {fname}: {len(remaining)} lines remaining ===")
    # Categorize
    comments = [(n,l) for n,l in remaining if re.search(r"^\s*(?://|/?\*)", l.lstrip())]
    strings = [(n,l) for n,l in remaining if re.search(r'["\'\`].*[\u4e00-\u9fff]', l) and not re.search(r"^\s*(?://|/?\*)", l.lstrip())]
    other = [(n,l) for n,l in remaining if (n,l) not in comments and (n,l) not in strings]
    print(f"  Comments: {len(comments)}, Strings: {len(strings)}, Other: {len(other)}")
    print(f"\n  Sample comments (first 10):")
    for n, l in comments[:10]:
        print(f"    L{n}: {l.strip()[:100]}")
    print(f"\n  Sample strings (first 10):")
    for n, l in strings[:10]:
        print(f"    L{n}: {l.strip()[:100]}")
    if other:
        print(f"\n  Sample other (first 5):")
        for n, l in other[:5]:
            print(f"    L{n}: {l.strip()[:100]}")

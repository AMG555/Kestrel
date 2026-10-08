#!/usr/bin/env python3
"""Show remaining Chinese patterns in JS files for analysis."""
import re, os

all_remaining = {}

for root, dirs, files in os.walk("web/static/js"):
    dirs[:] = [d for d in dirs if d != "vendor"]
    for fname in sorted(files):
        if not fname.endswith((".js", ".cjs")):
            continue
        path = os.path.join(root, fname)
        with open(path, "r", encoding="utf-8", errors="replace") as f:
            lines = f.readlines()
        
        for i, line in enumerate(lines):
            if re.search(r"[\u4e00-\u9fff]", line):
                # Extract Chinese strings/phrases
                # Find all Chinese runs (with surrounding context)
                spans = re.findall(r'[\u4e00-\u9fff][^\u4e00-\u9fff\n]{0,30}', line)
                for span in spans:
                    key = span.strip()
                    if key not in all_remaining:
                        all_remaining[key] = []
                    all_remaining[key].append(f"{fname}:L{i+1}")

# Sort by frequency
freq = sorted(all_remaining.items(), key=lambda x: -len(x[1]))
print(f"Unique Chinese patterns: {len(freq)}")
print("\nTop 100 most common patterns:")
for pattern, locations in freq[:100]:
    print(f"  [{len(locations):3d}x] {pattern[:60]}")

#!/usr/bin/env python3
"""Extract all remaining Chinese lines from JS files and save to a translation work file."""
import re, os, json

lines_by_file = {}

for root, dirs, files in os.walk("web/static/js"):
    dirs[:] = [d for d in dirs if d != "vendor"]
    for fname in sorted(files):
        if not fname.endswith((".js", ".cjs")):
            continue
        path = os.path.join(root, fname)
        with open(path, "r", encoding="utf-8", errors="replace") as f:
            file_lines = f.readlines()
        
        remaining = []
        for i, line in enumerate(file_lines):
            if re.search(r"[\u4e00-\u9fff]", line):
                remaining.append((i+1, line.rstrip()))
        
        if remaining:
            lines_by_file[path] = remaining

# Save to work file
with open("_remaining_lines.json", "w", encoding="utf-8") as f:
    json.dump({k: v for k, v in sorted(lines_by_file.items(), 
               key=lambda x: -sum(len(re.findall(r"[\u4e00-\u9fff]", l)) for _, l in x[1]))}, 
              f, ensure_ascii=False, indent=2)

# Print summary per file
total = 0
for path, lines in sorted(lines_by_file.items(), key=lambda x: -len(x[1])):
    chars = sum(len(re.findall(r"[\u4e00-\u9fff]", l)) for _, l in lines)
    total += chars
    print(f"{chars:5d} chars in {len(lines):4d} lines: {path}")

print(f"\nTotal remaining: {total} chars")

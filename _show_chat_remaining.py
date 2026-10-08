#!/usr/bin/env python3
"""
Read remaining Chinese JSON and print actual lines for the largest files
so we can see exactly what needs manual translation.
"""
import re, json

with open("_remaining_lines.json", "r", encoding="utf-8") as f:
    data = json.load(f)

# Print the remaining lines for the first 3 largest files
files_sorted = sorted(data.items(), key=lambda x: -sum(len(re.findall(r"[\u4e00-\u9fff]", l)) for _, l in x[1]))

for path, lines in files_sorted[:1]:  # Just chat.js first
    chars = sum(len(re.findall(r"[\u4e00-\u9fff]", l)) for _, l in lines)
    print(f"\n=== {path} ({chars} chars, {len(lines)} lines) ===")
    for no, line in lines[:100]:
        stripped = line.strip()
        zh_parts = re.findall(r"[\u4e00-\u9fff][^'\"\n]{0,50}", line)
        print(f"L{no:5d}: {stripped[:140]}")

#!/usr/bin/env python3
"""Analyze JS files to understand what Chinese patterns are present."""
import re

with open("web/static/js/chat.js", "r", encoding="utf-8") as f:
    content = f.read()
    lines = content.splitlines()

comment_lines = []
string_lines = []
other_lines = []
for i, line in enumerate(lines):
    if not re.search(r"[\u4e00-\u9fff]", line):
        continue
    stripped = line.strip()
    if stripped.startswith("//") or stripped.startswith("*") or stripped.startswith("/*"):
        comment_lines.append((i+1, line.rstrip()[:130]))
    elif re.search(r'["\'\`].*[\u4e00-\u9fff].*["\'\`]', line):
        string_lines.append((i+1, line.rstrip()[:130]))
    else:
        other_lines.append((i+1, line.rstrip()[:130]))

print(f"chat.js - Comments: {len(comment_lines)}, Strings: {len(string_lines)}, Other: {len(other_lines)}")
print("\n--- SAMPLE STRINGS (first 30) ---")
for no, line in string_lines[:30]:
    print(f"  L{no}: {line}")
print("\n--- SAMPLE COMMENTS (first 15) ---")
for no, line in comment_lines[:15]:
    print(f"  L{no}: {line}")
print("\n--- SAMPLE OTHER (first 10) ---")
for no, line in other_lines[:10]:
    print(f"  L{no}: {line}")

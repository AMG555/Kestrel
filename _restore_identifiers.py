import subprocess, re, glob, os

head_names = set()
fn_pattern = re.compile(r'(?:function\s+|window\.|\bclass\s+)([A-Za-z0-9_$]+)')

for f in sorted(glob.glob('web/static/js/*.js')):
    git_path = f.replace('\\', '/')
    out = subprocess.run(['git', 'show', f'53f75f4:{git_path}'], capture_output=True, text=True, encoding='utf-8')
    if out.returncode == 0:
        for m in fn_pattern.findall(out.stdout):
            if len(m) > 3 and any(c.isupper() for c in m):
                head_names.add(m)

print(f'Total CamelCase/PascalCase identifiers from HEAD: {len(head_names)}')

# Create lowercase to original map
lower_map = {name.lower(): name for name in head_names}

# Find in current files where a name is used with wrong casing
changes_found = 0
all_files = sorted(glob.glob('web/static/js/*.js')) + sorted(glob.glob('web/static/js/*.test.cjs'))
for f in all_files:
    with open(f, 'r', encoding='utf-8') as fp:
        content = fp.read()
    orig = content
    for lower, proper in lower_map.items():
        # Check if the wrong casing exists in the file
        matches = re.findall(r'\b(' + re.escape(lower) + r')\b', content, re.IGNORECASE)
        for m in set(matches):
            if m != proper and m.lower() == lower:
                # Replace exact wrong casing m with proper
                content = re.sub(r'\b' + re.escape(m) + r'\b', proper, content)
    if content != orig:
        with open(f, 'w', encoding='utf-8') as fp:
            fp.write(content)
        changes_found += 1
        print(f'Restored identifiers in {f}')

print(f'Total files updated: {changes_found}')

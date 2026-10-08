import os, re

chinese_pattern = re.compile(r'[\u4e00-\u9fff]')

ignored_dirs = {'.git', 'node_modules', 'dist', 'bin', '.vscode', '.idea'}
ignored_exts = {'.exe', '.dll', '.bin', '.png', '.jpg', '.jpeg', '.gif', '.ico', '.woff', '.woff2', '.ttf', '.eot'}

results = {}

for root, dirs, files in os.walk('.'):
    dirs[:] = [d for d in dirs if d not in ignored_dirs]
    for file in files:
        if file.startswith('_') or file.endswith('.txt') or file.endswith('.jsonl'):
            continue
        ext = os.path.splitext(file)[1].lower()
        if ext in ignored_exts:
            continue
        filepath = os.path.normpath(os.path.join(root, file))
        try:
            with open(filepath, 'r', encoding='utf-8', errors='ignore') as fp:
                lines = fp.readlines()
            matches = []
            for i, line in enumerate(lines):
                if chinese_pattern.search(line):
                    matches.append((i + 1, line.strip()))
            if matches:
                results[filepath] = matches
        except Exception:
            pass

print(f"Total files with Chinese characters: {len(results)}\n")
for f, matches in sorted(results.items(), key=lambda x: -len(x[1])):
    print(f"{f}: {len(matches)} lines")

import json, glob, re

with open('web/static/i18n/en-US.json', 'r', encoding='utf-8') as fp:
    i18n = json.load(fp)

all_keys = set()
def collect_keys(d, prefix=''):
    for k, v in d.items():
        full = f'{prefix}.{k}' if prefix else k
        all_keys.add(full)
        if isinstance(v, dict):
            collect_keys(v, full)

collect_keys(i18n)
print('Total i18n keys:', len(all_keys))

lower_keys = {k.lower(): k for k in all_keys if any(c.isupper() for c in k)}

updated = 0
for f in sorted(glob.glob('web/static/js/*.js')):
    with open(f, 'r', encoding='utf-8') as fp:
        content = fp.read()
    orig = content
    for m in set(re.findall(r"['\"]([a-zA-Z0-9_\.]+)['\"]", content)):
        if m.lower() in lower_keys and m != lower_keys[m.lower()]:
            correct = lower_keys[m.lower()]
            content = content.replace(f"'{m}'", f"'{correct}'")
            content = content.replace(f'"{m}"', f'"{correct}"')
    if content != orig:
        with open(f, 'w', encoding='utf-8') as fp:
            fp.write(content)
        updated += 1
        print('Updated i18n keys in:', f)

print('Files updated:', updated)

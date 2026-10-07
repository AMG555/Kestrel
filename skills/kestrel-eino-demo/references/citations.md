# Citations and External Links (example)

This file is used to verify that the **`references/`** directory within the skill package is correctly recognized by the listing API, HTTP `resource_path`, and multi-agent native file tools.

## How to Test (within an authorized environment)

1. The `GET /api/skills/kestrel-eino-demo` response's `package_files` should include `references/citations.md`.
2. `GET /api/skills/kestrel-eino-demo?resource_path=references/citations.md` should return the content of this file.
3. In multi-agent sessions with `eino_skills.filesystem_tools` enabled, this file can be read via relative path.

## Placeholder Citations

- [OWASP Testing Guide](https://owasp.org/www-project-web-security-testing-guide/) (link format example only)

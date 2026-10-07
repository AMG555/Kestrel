# Skills Directory (Agent Skills / Eino)

- Each skill is a **subdirectory**; the root must contain a **`SKILL.md`** file (YAML front matter: `name`, `description` + Markdown body) — see [agentskills.io](https://agentskills.io/specification.md).
- **The directory name must match `name`**.
- **Runtime loading**: In **Eino DeepAgent (multi-agent)** sessions the ADK **`skill` middleware** progressively discloses skills (each skill's name/description is listed in the system prompt; the model then calls the **`skill`** tool to fetch the full `SKILL.md`). Optionally enable **`multi_agent.eino_skills.filesystem_tools`** to use the same `read_file` / `execute` tools as the host machine to access scripts and resources inside the package.
- **Web management**: HTTP `/api/skills/*` is still used for listing, editing, and uploading files inside packages (implemented as `internal/skillpackage`, not MCP).
- **Runtime**: In multi-agent (DeepAgent) sessions, progressively loaded by the ADK **`skill`** tool. Single-agent MCP loops do not include Skills — enable multi-agent mode or use the future single-agent Eino path.

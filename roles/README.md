# Role Configuration Reference

This directory contains all role configuration files. Each role defines the AI's behavior mode and available tools.

## Creating a New Role

To create a new role, create a YAML file under the `roles/` directory in the following format:

**Method 1: Explicitly specify tool list (recommended)**
```yaml
name: role-name
description: role description
user_prompt: user prompt (prepended to user messages to guide AI behavior)
icon: "icon (optional)"
tools:
    # Add the tools you need...
    # ⚠️ Important: it is recommended to include the following core built-in MCP tools (vulnerability and knowledge base)
    - record_vulnerability
    - list_knowledge_risk_types
    - search_knowledge_base
enabled: true
```

**Method 2: Do not set the tools field (use all enabled tools)**
```yaml
name: role-name
description: role description
user_prompt: user prompt (prepended to user messages to guide AI behavior)
icon: "icon (optional)"
# Not setting the tools field defaults to all tools enabled in MCP management
enabled: true
```

## ⚠️ Important: Core Built-in MCP Tools

**If you set the `tools` field, make sure to include the following tools in the list (at least these three):**

1. **`record_vulnerability`** - Vulnerability management tool for recording discovered vulnerabilities
2. **`list_knowledge_risk_types`** - Knowledge base tool that lists available risk types
3. **`search_knowledge_base`** - Knowledge base tool for searching knowledge base content

You may also add WebShell, batch tasks, and other built-in or external tools as needed (subject to what is enabled in MCP management).

**Skills (skill packages)**: in **multi-agent / Eino** sessions, they are loaded on demand by the built-in **`skill`** tool from the `skills_dir` packages — no binding to role YAML.

**Note**: if you do not set the `tools` field, the system defaults to all tools enabled in MCP management. To explicitly control available tools for a role, it is recommended to set the `tools` field explicitly.

## Role Configuration Field Reference

- **name**: role name (required)
- **description**: role description (required)
- **user_prompt**: user prompt, prepended to user messages to guide the AI toward specific testing methods and focus areas (optional)
- **icon**: role icon, supports Unicode emoji (optional)
- **tools**: tool list specifying which tools this role can use (optional)
  - **If `tools` is not set**: all tools enabled in MCP management are selected by default
  - **If `tools` is set**: only tools in the list are used (recommend including at least the core built-in tools above)
- **enabled**: whether this role is enabled (required, true/false)

## Examples

Refer to other role files in this directory, such as `penetration-test.yaml`, `web-app-scan.yaml`, etc.

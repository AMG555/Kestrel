# Screenshots

This directory holds UI screenshots used in the root README.

## Naming convention

| File | Content |
|------|---------|
| `dashboard.png` | Main dashboard — stats, recent activity |
| `agent-chat.png` | Agent chat with streaming tool calls |
| `attack-chain.png` | SVG attack-chain graph for a project |
| `workflows.png` | Workflow graph editor |
| `hitl-queue.png` | Human-in-the-loop approval queue |
| `audit-log.png` | Append-only audit log table |

## How to add

1. Run Kestrel locally: `make all && ./build/kestrel`
2. Navigate to the page you want to screenshot
3. Take a screenshot at **1280 × 800** or wider
4. Save as a PNG to this directory using the name above
5. Update the README Screenshots section:

```markdown
## Screenshots

| Dashboard | Agent Chat |
|-----------|-----------|
| ![Dashboard](docs/screenshots/dashboard.png) | ![Agent](docs/screenshots/agent-chat.png) |

| Attack Chain | Workflows |
|-------------|-----------|
| ![Attack Chain](docs/screenshots/attack-chain.png) | ![Workflows](docs/screenshots/workflows.png) |
```

6. Commit: `git add docs/screenshots/ README.md && git commit -m "docs: add UI screenshots"`

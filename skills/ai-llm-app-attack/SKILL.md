---
name: ai-llm-app-attack
description: >-
  AI/LLM application attacks: prompt injection, agent tool abuse RCE, RAG poisoning, MCP supply chain, torch.load pickle RCE. Use when testing LLM apps, agents, RAG, MCP plugins, or AI model file risks.
metadata:
  tags: [penetration-testing, red-team]
---

## AI / LLM Application Attacks

```
=== AI/LLM Applications (real attack surface during the large-model application boom) ===
Prompt injection: Direct (ignore previous context, output system prompt) | Indirect (more dangerous): instructions hidden in RAG documents / web pages / emails / tool return values / image EXIF → hijack Agent
🚨Agent tool abuse (highest risk, direct path to RCE): code interpreter → inject execution | fetch tool → SSRF to intranet/cloud metadata | file tool → read /etc/passwd, write webshell
  | SQL tool → dump full table | shell tool → command injection → validation: actually trigger tool side effects (OOB callback / read file) before writing Fact
System prompt leak / RAG poisoning / over-privileged cross-tenant escalation / MCP plugin supply chain / resource cost attacks (burning tokens) | Output processing: LLM output fed into eval/SQL/frontend → secondary injection / stored XSS
Model files: torch.load uses pickle by default → RCE | Discover endpoints: capture traffic to find /chat /agent /tool, ask Agent "what tools do you have?"
```

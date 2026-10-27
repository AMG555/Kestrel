---
name: cicd-pipeline-security
description: >-
  CI/CD pipeline and developer infrastructure security auditing: GitHub Actions workflow injection (untrusted event data ${{ github.event.issue.title }}, pull_request_target misuse, composite action poisoning), Jenkins Groovy sandbox bypass and script console RCE, GitLab CI variable leakage and runner privilege escalation, runner compromise and persistence, OIDC cloud federation token abuse (AWS/GCP/Azure trust policies), and build artifact poisoning. Use when auditing CI/CD pipelines, DevOps workflows, runner hosts, or supply chain security.
metadata:
  tags: [cicd, supply-chain, devsecops, pipeline-security]
---

## CI/CD Pipeline & Supply Chain Security Auditing Playbook

```
=== Phase 1: GitHub Actions Workflow Vulnerability Auditing ===
1. Expression Injection in Inline Scripts (Untrusted Context):
   Vulnerable pattern: ${{ github.event.<context> }} rendered directly inside `run:` block:
     run: |
       echo "Processing issue: ${{ github.event.issue.title }}"
   Exploitation: Title set to `title" \n id \n echo "` -> executes arbitrary shell command on runner!
   Untrusted contexts:
     github.event.issue.title / body | github.event.pull_request.title / body
     github.event.comment.body | github.head_ref | github.event.head_commit.message
   Remediation verification:
     Check if untrusted context is passed safely via environment variables:
       env:
         TITLE: ${{ github.event.issue.title }}
       run: echo "Processing issue: $TITLE"
2. pull_request_target Privilege Escalation:
   Mechanism: runs in context of base repo with read/write GITHUB_TOKEN and repository secrets, triggered by PR from fork.
   Dangerous flaw: checking out untrusted PR head commit in pull_request_target:
     - uses: actions/checkout@v4
       with:
         ref: ${{ github.event.pull_request.head.sha }}
     - run: npm test # executes attacker-controlled package.json / scripts with base repo secrets!
3. GITHUB_TOKEN Scope & Permissions Check:
   Default permissions: if `permissions:` block missing in workflow, inspect repo settings (read-only vs permissive read-write).
   Tainted token extraction:
     curl -H "Authorization: token $GITHUB_TOKEN" https://api.github.com/repos/$GITHUB_REPOSITORY
     If `contents: write`: commit malicious backdoors to repo branches or tags.
     If `packages: write`: poison published container images or npm packages.
4. Composite Actions & Supply Chain Pinning:
   Verify action pinning: actions referenced by mutable tags (e.g. `uses: actions/checkout@v4` or `@main`) vs immutable commit SHA (`@b4ffde...`).
   Detect hijacked third-party composite actions: search for references to unregistered or deprecated GitHub marketplace accounts.

=== Phase 2: GitLab CI & Jenkins Auditing ===
1. GitLab CI Architecture Checks:
   Variables & Masking:
     Audit `.gitlab-ci.yml` for exposed CI/CD variables in build logs or artifact uploads.
     Protected variables: only available on protected branches/tags; check if untrusted merge request can trigger pipeline on protected branch.
   Runner Escape:
     Check runner executor (`docker`, `shell`, `kubernetes`, `custom`).
     If executor is `shell`: build jobs run directly on runner host OS with runner service user permissions.
     If privileged docker executor (`privileged = true` in config.toml): container breakout to runner host.
2. Jenkins Controller & Agent Exploitation:
   Script Console RCE:
     Access to `/script` (admin) or `/scriptText` API -> Jenkins Groovy execution:
       println "id".execute().text
   Groovy Sandbox Bypass:
     AST transforms, MetaClass modification, or type-confusion in pipeline scripts.
   Jenkins Credential Store Extraction:
     com.cloudbees.plugins.credentials.SystemCredentialsProvider.getInstance().getCredentials().each {
       if (it instanceof com.cloudbees.plugins.credentials.impl.UsernamePasswordCredentialsImpl) {
         println "${it.id}: ${it.username} / ${it.password.plainText}"
       }
     }

=== Phase 3: Cloud OIDC Federation & Secret Exfiltration ===
1. OIDC Token Exchange Exploitation:
   Modern CI/CD uses OpenID Connect (OIDC) to assume AWS/GCP/Azure roles without static credentials.
   Audit trust policy:
     Check `sub` (subject claim) condition:
       Loose condition: `repo:org/*:*` allows ANY repository in the organization to assume administrative cloud role!
       Proper condition: strict repo and branch pinning (`repo:org/repo:ref:refs/heads/main`).
   Extracting OIDC JWT in compromised runner:
     ACTIONS_ID_TOKEN_REQUEST_URL + ACTIONS_ID_TOKEN_REQUEST_TOKEN -> query JWT and exchange against AWS STS / GCP STS.
2. Build Cache & Artifact Poisoning:
   Actions cache abuse: actions/cache with permissive keys allows poisoning cached dependencies (node_modules, pip cache, maven repo).
   Artifact poisoning: if downstream jobs consume untrusted artifacts without checksum verification, attacker injects trojaned binary.
```

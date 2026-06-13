# Workflow: Dev Container Personas

Two dev container configurations exist in `.devcontainer/`. Choose the persona
that matches the operator — a human developer or an AI agent.

---

## Personas at a Glance

| | Human (`.devcontainer/`) | Agent (`.devcontainer/agent/`) |
|---|---|---|
| **Purpose** | Full interactive development | AI agent workloads (planning, analysis, code generation) |
| **AWS access** | `~/.aws` mounted read-write; full operator credentials | `~/.aws` mounted **read-only**; `agent-shell` assumes `itsjennyfiggy-agent-development` role at login |
| **terraform apply** | Permitted (subject to standard review gates) | **BLOCKED** by `.agents/bin/terraform` wrapper |
| **terraform plan** | Permitted | Permitted |
| **git auth** | `~/.gitconfig` mounted; uses host SSH/keychain | GitHub App token via `.agents/bin/git-auth`; no personal SSH keys mounted |
| **`AGENT_SHELL`** | Unset | `AGENT_SHELL=1` (set as `containerEnv` default) |
| **PATH prepend** | Standard system PATH | `.agents/bin` prepended via `postStartCommand` so wrappers shadow real binaries |

---

## Permission Matrix

### Human persona

| Resource | Permission |
|---|---|
| AWS (full operator role) | Read + Write + Destructive (scoped to operator IAM user/role) |
| terraform plan | Allowed |
| terraform apply / destroy | Allowed |
| git push to main | Blocked by branch protection; PR required |
| Personal SSH keys | Available via host keychain mount |

### Agent persona

| Resource | Permission |
|---|---|
| AWS (`itsjennyfiggy-agent-development` assumed role) | Read-only plan calls only |
| terraform plan | Allowed |
| terraform apply / destroy | **Blocked** — exits 1 with an explanatory message |
| git push to main | Blocked by branch protection; PR required |
| Personal SSH keys | Not mounted — use GitHub App token via `.agents/bin/git-auth` |
| Destructive AWS actions | **Hard-denied** by IAM `itsjennyfiggy-agent-deny-destructive` policy |

---

## Opening the Human Container

```bash
# In VS Code: "Dev Containers: Open Folder in Container"
# -> select .devcontainer/devcontainer.json  (the root, i.e. default)

# Or from the CLI:
devcontainer open .
```

The container mounts `~/.aws` (read-write) and `~/.gitconfig`, giving the
developer full credential access identical to their host workstation.

---

## Opening the Agent Container

```bash
# In VS Code: "Dev Containers: Open Folder in Container"
# -> select .devcontainer/agent/devcontainer.json

# Or target it explicitly:
devcontainer open --workspace-folder . --config .devcontainer/agent/devcontainer.json
```

On container start, `.agents/bin` is prepended to PATH so the terraform
intercept wrapper is always active. To begin an AWS-authenticated session with
the restricted role, run inside the container:

```bash
.agents/bin/agent-shell
# or pass a single command:
.agents/bin/agent-shell terraform plan
```

`agent-shell` calls `sts:AssumeRole` using the read-only `~/.aws` credentials,
then launches a clean subshell with only the temporary restricted credentials
exported — no ambient SSO tokens or profile variables leak through.

---

## GitHub App Git Authentication (Agent Persona)

Agents must not use personal SSH keys. Instead, configure the GitHub App token
before any `git push` or `gh` command:

```bash
# Inside the agent container, authenticate via the GitHub App wrapper:
PATH="$(pwd)/.agents/bin:$PATH" git push origin feat/my-branch
```

See `.agents/bin/git-auth` and `.agents/workflows/git-workflow.md` for the
full authentication setup.

---

## Updating the Base Image

Both personas use `mcr.microsoft.com/devcontainers/base:2.0.5-ubuntu-24.04`.

Before bumping the image tag, verify the latest stable release against the
primary source:

```
https://mcr.microsoft.com/v2/devcontainers/base/tags/list
```

Update both `devcontainer.json` files atomically and run the bats test suite
(`bats tests/`) to confirm the agent devcontainer JSON still validates.

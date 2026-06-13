# Workflow: agent-shell — Credential Shielding for AI Agents

## What agent-shell does

`.agents/bin/agent-shell` launches a credential-shielded subshell (or runs a
single command) for AI agents. It:

1. Uses the operator's **ambient AWS credentials** to call `sts:AssumeRole`
   against the restricted `itsjennyfiggy-agent-development` role.
2. Captures the short-lived temporary credentials returned by STS.
3. Calls `env -i` to construct a **clean environment** — no ambient
   `AWS_PROFILE`, SSO cache hints, or inherited session tokens leak through.
4. Exports **only** the temporary restricted credentials plus minimal
   system variables (`HOME`, `TERM`, `LANG`, `PATH`, `AWS_DEFAULT_REGION`).
5. Sets `AGENT_SHELL=1` so downstream tooling knows it is running inside a
   shielded context.

The session duration is **4 hours** (matching the role's `max_session_duration`).

## The role it assumes

`arn:aws:iam::940278682268:role/itsjennyfiggy-agent-development`

This role has two policies attached (provisioned via Terraform in BOOT-8):

| Policy | Effect |
|---|---|
| `itsjennyfiggy-agent-plan-only` | Allow read-only AWS calls sufficient for `terraform plan` |
| `itsjennyfiggy-agent-deny-destructive` | Explicit Deny on all write/delete actions (ec2:TerminateInstances, s3:PutObject, iam:Create*, iam:Delete*, etc.) |

The deny policy acts as a hard safety wall — even if a future policy
attachment accidentally grants broader permissions, the Deny wins.

## AGENT_SHELL=1 blocks terraform apply/destroy

`.agents/bin/terraform` is a wrapper that sits ahead of the real `terraform`
binary in PATH (because `agent-shell` prepends `.agents/bin/` to PATH). When
`AGENT_SHELL=1`:

- `terraform apply` → **BLOCKED** (exit 1)
- `terraform destroy` → **BLOCKED** (exit 1)
- `terraform plan`, `terraform plan -out=apply.tfplan` → allowed
- `terraform -chdir=x apply` → **BLOCKED** (flags are skipped; `apply` is
  still detected as the first non-flag subcommand)

Applies run only via CI on human-merged `main` per Zone A governance
(`.agents/rules/governance.md`).

## Usage

```bash
# Interactive shielded shell
.agents/bin/agent-shell

# Run a single command inside the shield
.agents/bin/agent-shell terraform plan

# Override the role ARN (e.g., for a different account)
AGENT_ROLE_ARN=arn:aws:iam::123456789012:role/my-role .agents/bin/agent-shell
```

## Sync path

These scripts live in `template-base/.agents/bin/` and are distributed to
consumer repositories via the existing `.agents/` base-sync workflow
(`figgy-base-sync` topic). Any repo that inherits from `template-base` will
receive updates to these scripts on the next sync run.

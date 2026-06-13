# ADR 0002 — GitHub App Identity Strategy

- **Status:** Accepted
- **Date:** 2026-06-07
- **Deciders:** Jenny Figgy
- **Related work:** BOOT-7 (GitHub App auth), BOOT-12 (release pipeline), ADR-0001

## Context

The release pipeline established in ADR-0001 requires a GitHub App installation token to function end-to-end. The default `GITHUB_TOKEN` cannot trigger downstream `on: release: published` workflows by design — release-please tags and GitHub Releases created with it will not fire image build pipelines. A real App token wired into release-please's `token:` input is therefore a hard prerequisite, not an optimization.

The org already operates three purpose-scoped GitHub Apps:

- **`figgy_bot`** — template sync across repos; org-wide install with broad write permissions (branches, PRs, merges).
- **`platform_admin`** — Terraform plan and apply operations.
- **`gitops_writer`** — ECR image builds and GitOps manifest writes.

Credentials for each are stored as SecureString in AWS SSM under `/itsjennyfiggy/global/<app>_github_app_*` and fetched in CI via OIDC — a pattern already proven in production.

The open question for BOOT-7: which App identity should drive the release pipeline, and how should it be structured to honor least privilege while remaining practical to operate across many small repos?

One hard constraint shapes the answer: **GitHub exposes no programmatic App creation path** — no API, no Terraform resource. Every GitHub App must be created manually through the org settings UI or the manifest flow. Any scheme that requires one app per repo is therefore impractical at scale.

## Decision Drivers

- Least privilege and minimal blast radius — a compromised release token must not reach beyond the releasing repo.
- Practical operability at scale — no per-repo manual App creation as the repo count grows.
- Clean separation of concerns and a clear audit trail — release events must be attributable to a single, well-named identity, not conflated with sync or infra operations.
- Consistency with the existing OIDC → SSM → scoped-token pattern already in use by the other three Apps.

## Considered Options

| Option | Summary | Decision |
|---|---|---|
| **Reuse `figgy_bot`** | Org-wide install; token already available in SSM. | **Rejected.** `figgy_bot` is over-permissioned for releases — its broad branch/PR/merge write access was designed for template sync, not releasing. A compromised release workflow token would reach far beyond its intended scope; the audit trail would conflate sync and release events, making incident analysis harder. |
| **One GitHub App per repo** | Tightest blast radius — each repo's token can only touch that repo. | **Rejected.** GitHub has no programmatic App creation API. Every new repo would require manual UI work in org settings. Unmanageable as the platform grows. |
| **One dedicated `figgy-release` app, org-installed, token scoped per-job** | New purpose-scoped App; token minted at job time restricted to the single releasing repo via `repositories:` input. | **Chosen.** Achieves least-privilege blast radius without the per-repo-app maintenance burden. Clean identity, clean audit trail. |

## Decision

Create a new GitHub App **`figgy-release`** with the minimum permissions release-please requires and nothing else:

- **Contents: Write** — create release commits, push tags, publish GitHub Releases.
- **Pull Requests: Write** — open, label, and merge the release PR.
- **Issues: Write** — apply release-please's `autorelease: pending` / `autorelease: tagged` labels.
- No admin, no code review, no organization members, no webhooks.

Install the app **org-wide** (matching the pattern of `figgy_bot`). At job time, the release workflow mints a token scoped to the single releasing repo by passing that repo name via the `repositories:` input of `actions/create-github-app-token`. Despite the org-wide install, the **effective blast radius per run is one repo's contents + PRs** — the token is cryptographically restricted to that scope by GitHub's App installation token machinery.

This achieves least-privilege token behavior without the impractical per-repo-app overhead that GitHub's UI-only App creation makes unworkable.

Credentials are stored as SecureString in AWS SSM at:

```
/itsjennyfiggy/global/figgy_release_github_app_id
/itsjennyfiggy/global/figgy_release_github_app_client_id
/itsjennyfiggy/global/figgy_release_github_app_private_key
```

This follows the same path tier and naming convention as `figgy_bot`. Note that SSM access control on this platform is enforced by per-ARN IAM grants on each consumer repo's OIDC role (`ssm_allowed_parameters`), not by path wildcards — so the `/global/` prefix signals lifecycle and ownership, not access scope. Each repo's OIDC role must be explicitly granted read access to the three figgy-release parameter ARNs before it can mint a token.

Per-repo onboarding after the one-time App creation is pure Terraform: update the repo's OIDC role with the three SSM parameter ARNs in `ssm_allowed_parameters`. No GitHub UI steps required per repo.

## Consequences

**Positive**

- Clean four-way separation across the App fleet: sync (`figgy_bot`), Terraform (`platform_admin`), GitOps (`gitops_writer`), release (`figgy-release`). Responsibilities never overlap.
- Release events are solely attributable to `figgy-release` — audit trails are unambiguous.
- Per-run tokens are cryptographically scoped to one repo, achieving least-privilege behavior despite an org-wide install.
- Reuses the proven OIDC → SSM → scoped-token mechanism; no new patterns to operate.
- Per-repo onboarding after initial setup is pure Terraform — no manual GitHub UI work per repo.

**Negative / costs**

- One-time **manual** App creation in GitHub org settings — no IaC path exists. A human must complete this step once before BOOT-7 is unblocked.
- A human must seed the three SSM parameters once via AWS Console or CLI after App creation.
- One additional App identity to track and rotate credentials for over the App's lifetime.

## References

- [ADR-0001 — Release Automation Strategy](0001-release-automation-strategy.md)
- [actions/create-github-app-token — token scoping via `repositories:`](https://github.com/actions/create-github-app-token)
- [GITHUB_TOKEN cannot trigger workflows (GitHub Docs)](https://docs.github.com/en/actions/concepts/security/github_token)

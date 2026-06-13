# ADR 0001 — Release Automation Strategy

- **Status:** Accepted
- **Date:** 2026-06-08
- **Deciders:** Jenny Figgy
- **Related work:** BOOT-7 (GitHub App auth), BOOT-12 (Semver tag action), BOOT-6 (Homelab template & multi-arch CD), BOOT-1 (Master bootstrapping checklist)

## Context

The platform produces several distinct kinds of versioned artifacts across many small repositories:

- **GitHub Actions** — reusable TypeScript/composite actions consumed by tag (`uses: owner/action@v1`).
- **Applications** — homelab/container apps published as multi-arch images to `ghcr.io`.
- **Infrastructure & Deployment** — Terraform and Kubernetes manifests delivered via GitOps.

Most code is now authored by agents opening pull requests. Agent output quality is still uneven, so today a human reviews and merges every feature PR. The desired end state is the inverse: a mature agent framework that handles its own review/merge, leaving the human to authorize *releases* only.

Two recurring frustrations motivated this decision:

1. **The "double merge" tax.** A naive release-please setup requires a human to merge the feature PR *and* then merge the bot's release PR — two manual steps to ship one change.
2. **Tooling indecision.** release-please vs semantic-release vs changesets, and whether actions and apps should share one approach.

## Decision Drivers

- Minimize human toil now, without re-platforming later.
- One mental model across artifact types where semver applies (consistency + maintainability + a coherent platform-engineering narrative).
- A smooth migration path as agent autonomy matures (move automation, not tools).
- Language-agnostic: many small polyglot repos; avoid forcing per-repo runtime config.
- Reviewable, human-readable changelogs and releases.

## Considered Options (tooling)

| Tool | Model | Why / Why not |
|---|---|---|
| **release-please** | Bot maintains a "release PR"; merging it cuts the release | **Chosen.** PR-based and reviewable; language-agnostic (runs in its own action, no runtime forced into repos); actively maintained (action `v5.0.0`, Apr 2026, node24); its "human merges the release PR" model *is* our target end state. |
| semantic-release | Fully automatic on each merge | Rejected. Node-centric — non-Node repos still need Node in CI plus `@semantic-release/exec`; tightly couples versions to commit messages; leaves `package.json` at `0.0.0`. Config sprawl across many repos. |
| changesets | Per-PR changeset file authored by the contributor | Rejected. The per-PR "pause" adds friction for agents; JS-monorepo-flavored. Wrong shape for many small polyglot repos. |

## Decision

### 1. Standardize on **release-please** wherever semver applies (apps + actions). 

Infrastructure and deployment stay on **GitOps** (plan-only Terraform, OIDC-gated apply, Argo promotion) — a separate lane, not release-please.

### 2. Separate three independent knobs.

The "double merge" pain is **not** the tool — it is *which gate is automated*. These are decided independently:

- **Knob A — Tool:** release-please (fixed, above).
- **Knob B — Which merge a human performs.** The release PR is a *batching/timing* gate, **not** a second review gate (review already happened on the feature PR). So it can be auto-merged.
- **Knob C — Per artifact type policy** (below).

### 3. Per-artifact-type policy.

| Artifact | Release engine | Human gate (now) | Migration target |
|---|---|---|---|
| **GitHub Actions** | release-please + **moving major tag** (`v1` → `vX.Y.Z`) | Feature PR only; **auto-merge the release PR** | Auto-merge feature PRs after agent review; human optional |
| **Applications** | release-please → release event triggers `ghcr.io` multi-arch build/push | **Human merges the release PR** (deliberate "ship it") | Auto-merge feature PRs; human keeps the release gate |
| **Infrastructure / Deployment** | GitOps (no release-please) | Human merge = plan/apply via OIDC | Unchanged |

### 4. The automation is a dial that flips over time — the tooling does not change.

- **Today** (agent output needs review): human merges feature PRs; the *redundant* release PR is auto-merged (actions) or human-merged (apps).
- **Mature state** (agentic review/merge trusted): feature-PR merges are automated; the human's only remaining action is authorizing the release (merge the release PR for apps).

### 5. Bot identity is a functional prerequisite, not just hygiene.

The default `GITHUB_TOKEN` cannot trigger downstream workflows by design. A release-please tag/release created with it will **not** fire the "build & push image on release" workflow. Therefore the release pipeline **must** run release-please (and tag updates) with a **GitHub App installation token** (via `actions/create-github-app-token`), wired into release-please's `token:` input.

This makes **BOOT-7** (the `itsjennyfiggy-platform-bot` GitHub App) a hard dependency of the release cascade:

> **BOOT-7** (App token) → **BOOT-12** (release workflow + `v1`-tag action) → **BOOT-6** (image publishing).

## Consequences

**Positive**

- One release tool and mental model across apps + actions.
- No double-merge tax today (auto-merged release PR for actions; single deliberate release for apps).
- Zero re-platforming to reach the mature state — only the automated gate moves.
- Clean separation: release-please for artifacts, GitOps for infra/deploy.
- A coherent, modern platform-engineering story (trunk-based, Conventional Commits, automated semver/changelog, release-gated deploys, OIDC, GitHub App identity).

**Negative / costs**

- Requires the BOOT-7 GitHub App token to be live before the cascade functions end-to-end.
- Conventional Commits discipline must be enforced on merges to `main` (agents and humans).
- The moving-major-tag behavior that Actions need is **not** provided by release-please and must be built (BOOT-12).

## Implementation Notes

- **BOOT-12** delivers (a) a TypeScript major-moving-tag action (built from `template-github-action`) that re-points `v1` via the GitHub refs API, and (b) a shared reusable release workflow wiring release-please + the BOOT-7 App token + the tag action.
- Auto-merge of release PRs is enabled via GitHub native auto-merge (or a workflow that enables it on the `autorelease: pending` label).
- `release-please-config.json` + `.release-please-manifest.json` are scaffolded into the relevant templates.

## References

- [release-please-action releases](https://github.com/googleapis/release-please-action/releases)
- [GITHUB_TOKEN cannot trigger workflows (GitHub Docs)](https://docs.github.com/en/actions/concepts/security/github_token)
- [actions/create-github-app-token](https://github.com/actions/create-github-app-token)
- [NPM release automation comparison](https://oleksiipopov.com/blog/npm-release-automation/)

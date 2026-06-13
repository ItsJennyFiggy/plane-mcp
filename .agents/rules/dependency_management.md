---
trigger: model_decision
description: Loaded during dependency addition, package upgrades, package manifest edits (package.json, requirements.txt, etc.), GitHub Actions workflow edits, Dockerfile/Docker Compose edits, Terraform provider configuration, or project bootstrapping to ensure package integrity and avoid outdated version references.
---

# Dependency Management & Integrity Rule

This rule governs how packages, libraries, Docker images, GitHub Actions, and Terraform providers/modules are added, updated, or removed in the workspace. It enforces safety gates to prevent dependency confusion, version/package name hallucination, outdated configurations, and licensing violations.

---

## 1. Anti-Hallucination & Version Grounding Mandate

*   **Registry Check Mandate**: Before editing any project dependency manifest file or referencing any external library:
    *   The agent MUST run a web search or check the official package registry for the active language/runtime to verify that the package name is correct and the package actively exists.
    *   Do **NOT** trust or install package names suggested by LLM training weights without verifying their existence.
*   **Version Verification**: Look up the latest stable major/minor version. Do not install outdated or deprecated releases unless explicitly requested due to a legacy constraint.

---

## 2. Multi-Component Version Grounding Standard

Whenever consuming, updating, or referencing versions of external components, the agent **MUST** run a web search to verify the latest stable version and must not rely on pre-existing knowledge or guess defaults:

*   **GitHub Actions**: Before creating or editing GitHub workflow files (`.github/workflows/*.yml`), the agent MUST verify the latest stable major/minor versions of all referenced actions (e.g., `actions/checkout`, `aws-actions/configure-aws-credentials`, `docker/setup-buildx-action`) and proactively upgrade them during edits to ensure pipeline security.
*   **Docker Images**: Before specifying base images in a `Dockerfile` or `docker-compose*.yml`, the agent MUST run a web search to check for the latest stable parent/runtime image tags (e.g., `node`, `python`, `postgres`, `alpine`).
*   **Terraform Providers & Modules**: Before declaring provider requirements (e.g., `hashicorp/aws`, `cloudflare/cloudflare`, `integrations/github`) or using external modules, verify the latest stable version tags.

---

## 3. Rigorous Verification Methodology (Anti-Staleness & Anti-Hallucination)

*   **Individual Focused Queries**: The agent **MUST NOT** run single, consolidated queries for multiple packages, actions, or images (e.g., searching for four actions at once). Multi-item searches frequently return outdated or aggregated summaries. Run individual, focused queries for each dependency.
*   **Primary Source Verification**: The agent **MUST** look beyond a search engine's synthesized summary block and cross-reference with primary source evidence, such as the specific GitHub release tags, official repository release pages, NPM/PyPI registries, or official changelogs, to guarantee that the version identified is active and truly the latest.
*   **Do NOT Year-Anchor Search Queries**: Appending a year (e.g., "2025") to a version search query will bias results toward older indexed pages from that year, returning *stale* data. **Always** search without a year anchor and instead navigate directly to the primary release page (e.g., `https://github.com/<owner>/<action>/releases`) to confirm the most recent release tag.

---

## 4. Lockfile Integrity & Synchronization

*   **Atomic Updates**: Every change to a dependency manifest file MUST be accompanied by an update to the corresponding lockfile.
*   **Immediate Synchronization**: Run the localized package manager installation or lock command immediately after changing a manifest to ensure the lockfile matches.
*   **No Manual Lockfile Editing**: Never modify lockfiles directly by hand. Always let the package manager generate them.

---

## 5. Vulnerability and Licensing Audits

*   **Security Scanning**: When adding a new dependency, run the language-specific audit command supported by the workspace's toolchain (e.g., dependency check, vulnerability scanning, or update checks).
*   **Restrictive License Prevention**: Inspect the license of any newly added package. Do **NOT** introduce copyleft or highly restrictive licenses (e.g., GPL, AGPL) unless explicitly approved by the user. Prefer permissive licenses like MIT, Apache 2.0, or BSD.

---
trigger: model_decision
description: Loaded during local environment configuration, toolchain setup, dev server execution, or database migrations to ensure safety and prevent orphaned background tasks.
---

# Rule: Environment Bootstrapping & Local Process Safety

This rule dictates how agents configure, run, and manage local dev environments and background services. The agent MUST follow this rule to prevent resource conflicts or corrupting host system configurations.

---

## 1. Dependency and Runtime Installation

*   **Localized Tooling Preferability**: Always use workspace-localized package managers where possible. Avoid global installations unless explicitly requested by the user.
*   **Host Environment Integrity**: Never alter the system-wide shell profiles or download unverified global binary executables directly into system paths.
*   **Version Pinning**: When installing new host CLI toolchains or language runtimes, check if a workspace tool-version config file exists and match the specified versions.

---

## 2. Port Management & Process Orphanage

Before launching a development server, mock service, or background API:

1.  **Port Availability Check**:
    Check if the target port is already in use using standard shell tools (e.g., `lsof -i :<port>` or `netstat`).
2.  **Conflict Resolution**:
    If the port is in use, report it to the user or request permission to bind to a different port. Do **NOT** kill existing processes blindly (`kill -9`) unless they are orphaned tasks spawned in a previous turn of the same agent.
3.  **Process Tracking & Cleanup**:
    *   Always launch dev servers using background task managers or native shell wrappers rather than infinite blocks.
    *   Do not leave orphaned mock databases, redis servers, or development pipelines running on the host system. Clean up background tasks immediately when execution is finished.

---

## 3. Database & Cache Seeding Safety

*   **Migration Ordering**: Always run database migrations sequentially and verify they complete successfully before running seeding or loading steps.
*   **Schema Safety**: Never bypass the application's ORM or migration framework to run direct raw SQL schema-altering commands unless explicitly instructed by the user.
*   **Reversible Seeding**: Mock data insertion scripts should be clean, repeatable, and idempotent. Avoid seeding duplicate rows or polluting shared databases.

---

## 4. AWS Credential Gotchas

To ensure seamless AWS authentication and avoid lockout during infrastructure deployment:

1.  **IAM Role Assumption Limit:**
    AWS root credentials cannot assume IAM roles. When executing local commands or shell scripts that assume a role (e.g., `itsjennyfiggy-agent-development`), the `agent-shell` environment MUST be configured with an IAM-principal source profile (not root credentials).
2.  **Local Terraform Provider Session:**
    The local Terraform provider does not automatically read the active `aws login` SSO session. You MUST export the current SSO credentials to the environment variables before running Terraform plans/applies:
    ```bash
    eval "$(aws configure export-credentials --format env)"
    ```
3.  **CI Role Policies and Planning-Stage Lockouts:**
    Before modifying or restricting permissions of any CI role, you MUST enumerate and preserve all necessary *refresh-read* permissions (e.g., `sns:GetTopicAttributes`, `s3:GetBucketAcl`). Removing these permissions can prevent the role from planning or correcting its own policy configuration, leading to lockout.


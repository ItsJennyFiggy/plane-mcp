---
trigger: model_decision
description: Loaded during git staging, committing, or pushing operations to enforce secret prevention, ban wildcard staging (git add .), and mandate automatic .gitignore shielding for temporary sensitive assets.
---

# Git Safety & Secrets Prevention Rule

To preserve the absolute security of this repository and prevent accidental exposure of cloud credentials, private keys, or API tokens on GitHub, you MUST strictly adhere to this safety rule.

---

## 🚫 1. Absolute Git Directives

*   **NEVER run `git add .` or `git commit .` or `git commit -a` blindly.** Staging all files in the working directory using wildcards is completely prohibited. You MUST surgically and explicitly stage files by their specific path (e.g., `git add infra/terraform/envs/dev/main.tf`).
*   **NEVER push commits directly to protected branches.** Pushing directly to `main`, `master`, `develop`, or `release/*` is strictly prohibited. All changes must be pushed to feature branches and merged via Pull Requests.
*   **NEVER push commits to a remote origin without performing a pre-push status audit.** You must always verify exactly what is staged and what changes are being committed.

---

## 🛡️ 2. Temporary Sensitive File Guardrails (.gitignore Shield)

Whenever you generate a temporary secret file, configuration file, PEM key, or local environment file in the workspace (such as `config-*`, `.env*`, `*.pem`, `*.key`):

1.  **Immediate `.gitignore` Append:**
    You **MUST** immediately append the exact file name or a matching wildcard pattern to the local `.gitignore` file **at the exact moment** you plan or initiate the file's creation.
    *   *Example:*
        ```bash
        echo "config-local-*" >> .gitignore
        ```
2.  **Verify Exclusion:**
    Run `git status` to ensure the sensitive file is marked as ignored and does not show up under "Untracked files".
3.  **Strict Deletion Workflow:**
    The file must remain ignored until it is completely deleted. Once deleted, you can safely remove the temporary pattern from `.gitignore` before making any clean commits.
4.  **Environment Template Synchronization:**
    When adding a new local configuration or environment variable to a `.env` file, you MUST simultaneously update the `.env.template` (or equivalent template configuration file) with placeholder/dummy values. This ensures the environment remains easily bootable for other developers.

---

## 📋 3. Staged Changes Pre-Commit Audit

Before you execute any `git commit` command, you MUST run:

1.  **Check Staged Files:**
    ```bash
    git status
    ```
    Review the "Changes to be committed" block to ensure only the intended files are listed.
2.  **Review the Diff:**
    ```bash
    git diff --cached
    ```
    Carefully inspect the actual lines of code being committed to guarantee that no hardcoded credentials, plain certificates, or private keys are captured.
3.  **Automated Security & Secret Audits:**
    If the repository has a pre-configured secrets scanner or static analysis auditor, you MUST run it locally before committing changes. For general repositories, run the standard security/vulnerability audit tool supported by the active package manager if applicable.

---

## ⚠️ 4. Error Correction Protocol
If you ever accidentally commit a sensitive asset locally:
1.  **STOP IMMEDIATELY.** Do **not** push to GitHub.
2.  Run `git reset --soft HEAD~1` to undo the commit while preserving your changes in the working tree.
3.  Unstage the file: `git restore --staged <file-name>`.
4.  Append the file to `.gitignore` and delete it from the disk.
5.  Re-commit only the secure changes.

---

## 🔐 5. Git Authentication & Remote Synchronization Rules

Whenever executing remote Git commands (such as `git pull`, `git fetch`, or `git push`):

1.  **Sync Success Mandate**: You **MUST** verify that every remote Git operation completes successfully. Never ignore a remote synchronization error and proceed to coding or branching.
2.  **Token Collision Resolution**:
    If a remote command fails with authentication errors (e.g., `Invalid username or token. Password authentication is not supported`):
    -   **Standard Repositories**: If the repository does not use a GitHub App, this error is typically caused by an invalid or expired `GH_TOKEN` or `GITHUB_TOKEN` environment variable inherited from the active sandboxed session. You **MUST** retry the command with those variables unset:
        ```bash
        GH_TOKEN="" GITHUB_TOKEN="" git pull origin main
        ```
    -   **GitHub App Repositories**: If the project has a `.env.git-app` configuration file present, you **MUST** configure the local `gh` wrapper using the `git-auth` skill, and prepend the wrapper's bin directory to the `PATH` for all git/gh CLI calls:
        ```bash
        PATH="$PWD/.agents/bin:$PATH" git pull origin main
        ```
3.  **App Configuration Shield**:
    If you configure GitHub App credentials in `.env.git-app` and `.env.git-app.pem`, you **MUST** immediately append those files to `.gitignore` to prevent committing private keys or app credentials to the repository.

---

## 🤝 6. Merge Policy & Authority

To prevent API failures and respect organization boundaries during pull request merges:

1.  **Squash-Merge Policy:**
    All repositories in this organization strictly enforce **squash-merge only**. You MUST use the squash option when merging via the CLI:
    ```bash
    gh pr merge --squash
    ```
    Attempting a standard merge (`--merge`) will fail at the GitHub API.

2.  **Authority Boundaries by Repository Type:**
    *   **Private Zone B Repositories:** You are permitted to self-merge your own pull requests once CI is green.
    *   **Public and Protected Repositories:** You MUST NOT self-merge. Explicit per-action user authorization is required. Use the admin bypass flag (`--admin`) only when the user has explicitly authorized the action for the current session.



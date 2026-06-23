# Workflow: Standard Git & PR Lifecycle

This workflow guides developers and agents through a safe, structured, and repeatable git lifecycle for task completion, ensuring code is validated, staged surgically, and review-ready.

---

## Step 1: Branch Scaffolding & Sync Verification
Before writing code, create a descriptive feature branch from the latest `main`. You **MUST** ensure your local `main` is successfully updated and synchronized with the remote:

1.  **Switch to main**:
    ```bash
    git checkout main
    ```
2.  **Pull the latest changes**:
    ```bash
    git pull origin main
    ```
3.  **Create the feature branch**:
    ```bash
    git checkout -b feature/your-feature-name
    ```
---

## Step 1.5: Plan Alignment (The Plan-First Gate)
For complex tasks or architectural modifications (e.g., editing database schemas, adding public endpoints, or changing configuration structures):
1.  **Draft Implementation Plan**: Create or update `implementation_plan.md` in the agent's scratch or artifacts directory.
2.  **User Review**: Request feedback and obtain explicit user approval before modifying codebase files.

---

## Step 2: Implementation & Validation
1.  **Test-Driven Development (TDD)**: Refer to `.agents/rules/testing_standards.md` to write tests mapping boundaries before implementing logic.
2.  **Code & Refactor**: Implement code in small chunks, running tests repeatedly.
3.  **Local Checks**: Run linters, formatters, and full test suites. If pre-commit hooks are configured (e.g., using the `pre-commit` tool), execute them locally to verify that the environment builds cleanly and no regressions exist.

---

## Step 3: Surgical Staging & Commit
1.  **Identify Changes**: Run `git status` to see modified and untracked files.
2.  **Surgical Stage**: Stage files explicitly by path. Do NOT run wildcard commands like `git add .` or `git add -A`.
    ```bash
    git add path/to/modified/file.py
    ```
3.  **Audit Diff**: Verify exactly what is staged to prevent credentials or temporary files from leaking:
    ```bash
    git diff --cached
    ```
4.  **Commit**: Commit with a clear semantic message (e.g. `feat(auth): add JWT validator`, `fix(db): resolve connection pool timeout`).
    ```bash
    git commit -m "feat(scope): descriptive summary"
    ```

---

## Step 3.5: Conflict Resolution Protocol
If the remote target branch has diverged during development:
1.  Fetch latest changes:
    ```bash
    git fetch origin main
    ```
2.  Rebase safely:
    ```bash
    git rebase origin/main
    ```
3.  If conflicts arise, resolve them file-by-file, update the staging area, and continue the rebase:
    ```bash
    git add <resolved-file>
    git rebase --continue
    ```
4.  Run the full test suite and validation scripts to ensure rebase stability.
5.  Never force-push (`git push -f`) unless explicitly approved by the user. Use `--force-with-lease` when updating remote branches.

---

## Step 4: PR Creation & Tracking
1.  **Push Branch**: Push the feature branch to the remote origin:
    ```bash
    git push origin feature/your-feature-name
    ```
2.  **Open Pull Request**: Use the GitHub CLI to open a Pull Request pointing to `main`:
    ```bash
    gh pr create --title "feat(scope): brief description" --body "Detailed summary of changes, testing results, and tickets closed."
    ```
3.  **Track CI/CD**: Monitor the GitHub Actions run to verify that all remote tests and compliance checks pass cleanly.

---
trigger: model_decision
description: Loaded when spawning secondary processes, delegating tasks, dividing coding and testing, or running parallel commands/workspaces to orchestrate subagent execution.
---

# Rule: Subagent Orchestration & Parallel Execution

This rule governs how tasks are decomposed and delegated to secondary processes or subagents. It defines tool-neutral roles and workspace isolation techniques to maximize concurrency while preventing file conflicts and context bloating.

---

## 1. Decision Criteria for Delegation

The agent should delegate work to a subagent under the following scenarios:
*   **Separable Sub-Tasks**: The task can be cleanly split (e.g., writing unit tests while concurrently implementing a feature).
*   **Context Isolation**: Deep reading of external documentation, massive API specifications, or large directories is required. Delegating research to a subagent prevents context window pollution in the main agent thread.
*   **Speculative Implementation**: Testing experimental features, new packages, or major refactorings that might need to be abandoned.

---

## 2. Tool-Neutral Subagent Personas

When defining or initiating a subagent, align its system configuration or instructions with one of these six standard personas:

1.  **Lead Coder**: Focuses strictly on writing or modifying application logic from a structured implementation plan.
2.  **Test Writer**: Focuses on writing deterministic unit and integration tests (TDD) with mock network interfaces.
3.  **Security Auditor**: Scans staging areas for vulnerabilities, credential leaks, and static analysis issues.
4.  **Code Reviewer (Validator)**: Read-only persona that reviews proposed changes and provides objective feedback.
5.  **Docs Sync**: Modifies only markdown documents, inline comments, or schemas to prevent "docs drift".
6.  **Researcher / Inspector**: Read-only persona that queries package registries and reads external docs.

---

## 3. Workspace & Runtime Isolation

Running concurrent subagents requires isolation to prevent state collisions:

*   **Git Workspace Isolation**:
    *   **Worktrees (Recommended)**: Use `git worktree` to check out separate directories for each subagent. This keeps individual runs isolated without duplicate repository cloning costs.
    *   **Isolated Branches**: Each subagent must work on its own feature branch (e.g., `feature/auth-tests`, `feature/auth-logic`) branched from the parent.
*   **Runtime & Process Sandboxing**:
    *   Never run concurrent subagents against the same shared local port or local database instance without dynamic mapping.
    *   Subagents running test suites should fall back to localized, in-memory databases (such as SQLite) or independent mock instances.

---

## 4. Concurrency Control & Merging

To prevent logical and physical merge conflicts ("merge tax"):

1.  **Strict File Ownership**: The main agent must assign strict, non-overlapping file or directory boundaries to concurrent subagents. Multiple agents must NEVER write to the same file at the same time.
2.  **Sequential Merging**: Merge changes back to the target branch sequentially.
3.  **Continuous Rebase**: When merging a completed subagent branch, pull latest changes, rebase the next subagent's branch, and run verification tests before merging.

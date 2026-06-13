---
description: Standardized workflow and guidelines for safe, reliable task tracking and board management via the Plane MCP.
---

# Workflow: Plane Task Tracking & Board Hygiene

This workflow governs how agents consume, assign, update, and track issues on the Plane Kanban board using MCP tools. Adhering to this workflow ensures repository progress and ticket state stay synchronized.

---

## 1. Task Assignment & Scopes

*   **Identities**: Retrieve tasks assigned to the active agent identity (typically `figgybot`).
*   **Projects**: Work is tracked under specific project identifiers (e.g., `AGENT` for Agent Infra, `CLI` for CloudCLI, `TMPL` for Templates).
*   **Scoping**: Query only the relevant state groups (e.g., `backlog`, `unstarted`, `started`) to locate outstanding items.

---

## 2. Status Transitions

When executing a task from start to finish, always reflect the status on the board using Plane MCP tools:

1.  **Select & Assign**:
    *   Find your assigned tasks in state `Todo` or `Backlog` (or transition them to `Todo` once selected).
    *   If using semantic tools, run `find_my_work` to locate assigned tickets.
2.  **In Progress**:
    *   Move the ticket's state to `In Progress` when starting implementation.
    *   Provide progress updates or post design decisions/implementation plans as comments using comment creation tools (e.g., `create_work_item_comment`).
3.  **In Review**:
    *   Move the ticket to `In Review` once code is written, locally verified, and a pull request has been opened.
    *   Link the pull request to the ticket using link creation tools (`create_work_item_link`).
4.  **Done**:
    *   Once the pull request is merged and verified, move the ticket to `Done`.

---

## 3. Safe Updates & Formatting Guidelines

*   **Description & Comment Format**: Plane uses a rich Tiptap-compatible editor. When creating or updating descriptions or comments via API, send valid HTML to `description_html` / `comment_html` and a plain text fallback to `description_stripped` / `comment_stripped`. Refer to the `plane-ticket-formatter` skill for the exact HTML markup tags.
*   **Non-Destructive Updates**: Prefer using specific, partial-update endpoints or fields. When performing a full update, ensure you preserve existing metadata (such as assignees, labels, and parent relationships) to avoid stripping them.

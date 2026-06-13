---
description: Standardized workflow and guidelines for safe, reliable task tracking and board management via the Vikunja MCP.
---

# Workflow: Vikunja Task Tracking & Board Hygiene

This workflow governs how agents consume, assign, update, and track issues on the Vikunja Kanban board using MCP tools. Adhering to this workflow prevents task data destruction and ensures absolute sync between repository progress and ticket state.

---

## 1. Task Assignment & Ownership

*   **Default Bot Assignee:** Always assign newly created or unassigned active tasks to `jenny_bot` (User ID `3`, username `JennyBot`).
*   **Human Owner:** The human owner is User ID `1`. Never reassign human tasks without permission.
*   **Collaborative Tracking:** Ensure all work items are assigned correctly to signal that a harness is actively processing the task.

---

## 2. Safe Update Recipe (Avoiding Full-PUT Overwrites)

> [!WARNING]
> **Destructive PUT Hazard:** The Vikunja MCP `tasks_update` command performs a destructive full-object replacement. Running it with partial arguments will silently delete other fields such as `description`, `done`, or assignee lists.

When you must update a task using `tasks_update`, you MUST follow this safe read-modify-write procedure:
1.  **Fetch First:** Retrieve the current task object by its ID using `tasks_get`.
2.  **Merge Properties:** Construct the update payload by merging the new/changed properties with all existing fields (including `description`, `done`, `bucket_id`, `project_id`, etc.).
3.  **Perform Update:** Call `tasks_update` with the complete merged payload.
4.  **Restore Assignees:** `tasks_update` might strip assignees. Re-add `jenny_bot` or relevant assignees immediately using `task_assignees_add` if they were removed.

---

## 3. Prefer Additive Operations

To minimize the risk of PUT-overwrite failures, prioritize specific, single-purpose additive tools over `tasks_update`:
*   **Move Tasks:** Use `task_move_to_bucket` to transition tasks between Kanban columns (e.g., to *Doing* or *Done*).
*   **Add Notes/Logs:** Use `task_comments_add` to post progress updates, handoffs, or build errors.

---

## 4. Handling Broken Done Filter

> [!IMPORTANT]
> **Done Filter Limitation:** The Vikunja list filter for `done = true` is broken and consistently returns `0` results. Completed tasks will also vanish from `done = false` lists.

To verify if a task was successfully completed or to read its state:
*   Do NOT rely on listing tasks and filtering by done state.
*   Instead, query the specific task directly by its unique ID using:
    ```bash
    tasks_get(id = <task-id>)
    ```

---

## 5. Status Transitions

When executing a task from start to finish, always reflect the status on the board:
1.  **Select & Assign:** Assign the task to `jenny_bot` and move it to the **Doing** bucket (Kanban view 64, Bucket ID 47).
2.  **Progress Comments:** Post comments with implementation plan links or command logs when significant sub-milestones are reached.
3.  **Completion:** Upon successful validation, move the task to the **Done** bucket (Bucket ID 48) and mark `done = true` following the safe update recipe.

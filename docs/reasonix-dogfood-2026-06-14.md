# plane-mcp dogfood log — Reasonix executor ticketing (2026-06-14)

Running record of `plane-mcp` (the custom Go MCP, formerly `plane-go`) gaps and
fallbacks hit while ticketing the Reasonix-as-executor evaluation and building the
`workstation/docker/reasonix-executor` image. Each gap is cross-linked to its Plane
ticket. "Fallback" = had to use the official `plane-fallback-mcp` (formerly `plane`)
because `plane-mcp` couldn't do it.

> Note: the MCP surface changed mid-session — `list_labels`, `add_label`, and
> `remove_label` were added to `plane-mcp`. Findings below mark what was fixed.

## Action surface — high vs low level (executor ↔ Plane)

| Action | Level | plane-mcp status (end of session) |
|---|---|---|
| `find_my_work` | High | present — but params over-required (GAP-1) |
| `get_work_item` | High | present — doesn't surface labels (GAP-7) |
| `report_progress` | High | present (not exercised) |
| `submit_for_review` | High | present (not exercised) |
| `create_task` | High | present — labels fixed (GAP-1b); still no parent (GAP-5) |
| `add_label` / `remove_label` | High | **added this session** — by-name, idempotent ✓ |
| `list_labels` | Low | **added this session** (was GAP-3) ✓ |
| `list_projects` / resolve project | Low | missing — fallback used (GAP-2) |
| set parent / sub-issue | Low | missing — fallback used (GAP-5) |

## Findings

### GAP-1 — `find_my_work` requires `project` + `state_group` → [AGENT-28]
Schema marks both `required`; description says they're optional filters. Re-checked
after the MCP update — still required. Breaks the executor's first dispatch call.
**Status: open.**

### GAP-1b — `create_task` dropped `labels` → [AGENT-21] — **FIXED (was stale)**
On the *pre-rename* MCP (`plane-go`), passing `labels` to `create_task` succeeded with
no error but the item (EXEC-6) came back `labels: []`. **Retested 2026-06-14 on the
updated `plane-mcp` in ASBX (ASBX-22):** labels passed by name are now resolved
server-side and attached (verified via fallback `retrieve_work_item_by_identifier` —
both `test:label` + `test:label2` present). The EXEC/AGENT tickets above were unlabeled
only because they were created on the old server before the update; relabeled via
`add_label`. AGENT-21 can be verified/closed.

### GAP-2 — no project enumeration / resolver → [AGENT-29]
Had to use `plane-fallback-mcp.list_projects` to discover EXEC/AGENT project ids.
`plane-mcp` has no way to enumerate or resolve projects. **Status: open.**

### GAP-3 — no label discovery → [AGENT-30] — **FIXED this session**
Originally had to use `plane-fallback-mcp.list_labels`. `plane-mcp.list_labels`
(project → id/name/color) was added mid-session. AGENT-30 is now implemented.

### GAP-5 — `create_task` has no `parent` → [AGENT-31]
Could not create EXEC-7..11 as real sub-issues of EXEC-6; created them flat, then
set parents via fallback `update_work_item.parent`. **Status: open.**

### GAP-6 — markdown GFM tables collapse in `create_task` description → [AGENT-32]
The high/low-level table in EXEC-6's description rendered as one collapsed line in the
stored HTML. **Retested 2026-06-14 on the updated `plane-mcp` (ASBX-23):** still
collapses — stored as `<p>| Col A | Col B | |---|---| ...</p>`, not a `<table>`.
Related to AGENT-19 (markdown→HTML) but table-specific. **Status: open (confirmed post-update).**

### GAP-7 — `get_work_item` doesn't surface labels → relates [AGENT-17]
Both `summary` and `detail` views return identical output with no labels field, so
label state can't be verified through `plane-mcp` at all (had to use the fallback).
**Status: open** (AGENT-17 is about tables/checklists; extend it to labels, or new ticket).

## Fallback-MCP interop note (not a plane-mcp bug)
`plane-fallback-mcp.update_work_item` / `create_work_item` could **not** set labels
through this agent harness: their `labels` param has no `type: array` in the schema,
so the harness serialized the array as a string and pydantic rejected it
(`list_type` error). This is a strong argument *for* `plane-mcp`'s by-name, server-side label resolution
(`add_label` single idempotent string arg; `create_task` now does the same for its
`labels` array) — it sidesteps the untyped-array harness problem that the fallback
MCP suffers from entirely.

## What worked well
- `add_label` — by-name, idempotent, clear success/no-op messages. Used to label all
  12 tickets after `create_task` dropped them.
- `create_task` — identifier assignment, priority, markdown body (sans tables), state
  defaulting to Backlog all clean.
- `ping`, `get_work_item` — fine for connectivity / titles.

## Live run validation (reasonix v1.7.0 → plane-mcp, 2026-06-14)

First real end-to-end exercise of the bridge: `reasonix run` (headless, `full`
profile, `deepseek-v4-pro` via OpenCode Zen Go) was asked to create a ticket and
clone a repo. Both succeeded autonomously:

- **create_task via the stdio plugin worked** — reasonix launched `plane-mcp` as a
  subprocess, env passed through (PLANE_API_KEY/BASE_URL/WORKSPACE_SLUG/PROFILE),
  and it created **ASBX-24**. Confirms the MCP bridge + env inheritance (EXEC-8).
- Used `PLANE_MCP_PROFILE=full` so `create_task` was exposed; still worth confirming
  the `worker` profile *excludes* it (AGENT-11 scoping).
- Cost/cache for the run: 10 steps · 105,216 cache-hit / 24,342 miss tokens
  (~81% hit) · ¥0.031 (~$0.004). Prefix-cache thesis holds even on a near-cold run.
- reasonix's own readiness guard blocked a premature final-answer once (open todo) —
  a useful built-in failsafe (relevant to the EXEC failsafe-wrapper work).

## Tickets created this session
- EXEC-6 (parent) + EXEC-7..11 (sub-tasks) + EXEC-12 (orchestrator stub) — `role:executor`/`role:orchestrator`.
- AGENT-28 (GAP-1), AGENT-29 (GAP-2), AGENT-30 (GAP-3, since implemented), AGENT-31 (GAP-5), AGENT-32 (GAP-6) — `repo:plane-mcp`.

# plane-mcp

A custom Go-native Model Context Protocol (MCP) server that integrates directly with the Plane.so REST API. It exposes clean, token-efficient, name-resolved tools to AI agents for automated task tracking, progress reporting, and workspace planning.

---

## 🎯 Features & Scope

*   **Stdio Transport**: Communicates natively over standard I/O (stdio) to plug into Claude Desktop or container runtimes.
*   **Tool Scoping Profiles**: Restricts the exposed tool surface dynamically via `PLANE_MCP_PROFILE=worker|planner|reviewer|full` (e.g., preventing worker agents from calling destructive or planning tools, or restricting reviewers to read + comment-back only).
*   **Token-Efficient Payload Serialization**: Serializes work items into compact formats, resolves UUID-valued fields (states, assignees, labels) to human-readable names, converts description HTML to Markdown, and strips nulls to optimize LLM context usage.
*   **Tier-1 Semantic Tools**: High-level, developer-oriented operations:
    *   `find_my_work`: Lists items assigned to the current caller.
    *   `get_work_item`: Fetches detail/summary view of a ticket.
    *   `list_work_items`: Lists work items in a project with optional filters.
    *   `search_work_items`: Searches work items across the workspace by text query.
    *   `report_progress`: Appends comments and transitions states safely in a single action.
    *   `submit_for_review`: Attaches PR links and flags tickets for review.
    *   `create_task`: Resolves labels/assignees by name and registers new tasks.
*   **Tier-2 CRUD Tools (Gated)**: Low-level operations (`update_work_item`, `list_comments`, `get_last_comment`, comment lists, link creation) registered only under the `planner` or `full` profiles.
*   **Off-Network Tunnel Ingress**: Supports Cloudflare Access service tokens via headers to route requests securely to local/private Plane deployments.

---

## 🛠️ Tech Stack & Architecture

*   **Runtime/Language**: Go 1.26
*   **Transport**: Model Context Protocol Go SDK (stdio)
*   **Build Packaging**: Multi-stage distroless scratch builds (`gcr.io/distroless/static-debian12`)
*   **CI/CD**: Automatic linting/testing via GitHub Actions, and container publication to GHCR via OIDC.

---

## 🚀 Getting Started

### Prerequisites

*   Go 1.26+ installed on host
*   A Plane.so workspace with a Personal Access Token (`PLANE_API_KEY`)

### Local Setup

1. Navigate to the repository directory:
   ```bash
   cd plane-mcp
   ```
2. Configure local environment variables:
   ```bash
   # Create a local .env configuration file
   export PLANE_API_KEY="your-plane-api-key"
   export PLANE_BASE_URL="http://192.168.86.32:3355" # or https://plane.figgy.cloud
   export PLANE_WORKSPACE_SLUG="itsjennyfiggy"
   export PLANE_MCP_PROFILE="full"
   ```
3. Install dependencies:
   ```bash
   go mod download
   ```
4. Run the application locally:
   ```bash
   go run ./cmd/app
   ```

### Running Tests

To run the local unit test suite and verify code coverage:

```bash
go test -v -race -cover ./...
```

---

## 🤖 AI Agent Guidelines

If you are an AI coding agent working in this repository:
1. Always audit the local `.agents/rules/` directory before making changes.
2. Adhere strictly to `.agents/rules/git_safety.md` to prevent secrets exposure.
3. Prior to presenting work or opening a PR, ensure all test suites pass and coverage gates are met (refer to `.agents/rules/testing_standards.md`).
4. Follow the standard branching and PR lifecycle detailed in `.agents/workflows/git-workflow.md`.

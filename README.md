# plane-mcp

[![Go Reference](https://pkg.go.dev/badge/github.com/ItsJennyFiggy/plane-mcp.svg)](https://pkg.go.dev/github.com/ItsJennyFiggy/plane-mcp)
[![Go Report Card](https://goreportcard.com/badge/github.com/ItsJennyFiggy/plane-mcp)](https://goreportcard.com/report/github.com/ItsJennyFiggy/plane-mcp)
[![CI](https://github.com/ItsJennyFiggy/plane-mcp/actions/workflows/ci.yml/badge.svg)](https://github.com/ItsJennyFiggy/plane-mcp/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/ItsJennyFiggy/plane-mcp)](https://github.com/ItsJennyFiggy/plane-mcp/releases/latest)
[![Container](https://img.shields.io/badge/ghcr.io-plane--mcp-blue?logo=docker)](https://github.com/ItsJennyFiggy/plane-mcp/pkgs/container/plane-mcp)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

A Go-native [Model Context Protocol](https://modelcontextprotocol.io) (MCP) server that wraps the [Plane.so](https://plane.so) REST API and exposes clean, token-efficient, name-resolved tools to AI agents over stdio — for automated task tracking, progress reporting, and workspace planning.

## Table of Contents

- [Features](#features)
- [Installation](#installation)
- [Quickstart](#quickstart)
- [Configuration](#configuration)
- [Tool Scoping Profiles](#tool-scoping-profiles)
- [Tools Reference](#tools-reference)
- [Building from Source](#building-from-source)
- [Contributing](#contributing)
- [Security](#security)
- [License](#license)

## Features

- **Stdio transport** — communicates over standard I/O to plug into Claude Desktop, Claude Code, or any MCP client.
- **Tool scoping profiles** — restrict the exposed tool surface via `PLANE_MCP_PROFILE=worker|planner|reviewer|full`, or pin an exact allowlist with `PLANE_MCP_TOOLS`. Keeps worker agents away from destructive/planning tools and limits reviewers to read + comment-back.
- **Token-efficient serialization** — work items are serialized to compact YAML, UUID-valued fields (states, assignees, labels) are resolved to human-readable names, description HTML is converted to Markdown, and nulls are stripped to save LLM context.
- **Name resolution everywhere** — projects, states, labels, modules, and members can be referenced by name, identifier, email, or ID; the server resolves them to UUIDs for you.
- **Off-network ingress** — supports Cloudflare Access service tokens via headers to reach private/self-hosted Plane deployments.

## Installation

Choose whichever fits your setup. The server is a single self-contained binary.

### `go install`

Requires [Go 1.26+](https://go.dev/dl/). Installs the `plane-mcp` binary into `$(go env GOBIN)` (or `$GOPATH/bin`):

```bash
# Latest release
go install github.com/ItsJennyFiggy/plane-mcp/cmd/plane-mcp@latest

# Or pin a specific version
go install github.com/ItsJennyFiggy/plane-mcp/cmd/plane-mcp@v1.13.0
```

Make sure that directory is on your `PATH` so MCP clients can find `plane-mcp`.

### Prebuilt binaries

Download a prebuilt archive for your OS/arch (linux, macOS, Windows; amd64, arm64) from the [latest release](https://github.com/ItsJennyFiggy/plane-mcp/releases/latest), extract it, and place the `plane-mcp` binary on your `PATH`.

### Docker (GHCR)

Multi-arch images are published to the GitHub Container Registry:

```bash
docker pull ghcr.io/itsjennyfiggy/plane-mcp:latest
```

## Quickstart

`plane-mcp` is an MCP server: an MCP client launches it and talks to it over stdio. Add it to your client's configuration with the standard `mcpServers` block.

### Claude Desktop / Claude Code (binary on `PATH`)

```json
{
  "mcpServers": {
    "plane": {
      "command": "plane-mcp",
      "args": [],
      "env": {
        "PLANE_API_KEY": "your-plane-api-key",
        "PLANE_BASE_URL": "https://plane.example.com",
        "PLANE_WORKSPACE_SLUG": "your-workspace-slug",
        "PLANE_MCP_PROFILE": "full"
      }
    }
  }
}
```

### Docker variant

```json
{
  "mcpServers": {
    "plane": {
      "command": "docker",
      "args": [
        "run", "-i", "--rm",
        "-e", "PLANE_API_KEY",
        "-e", "PLANE_BASE_URL",
        "-e", "PLANE_WORKSPACE_SLUG",
        "-e", "PLANE_MCP_PROFILE",
        "ghcr.io/itsjennyfiggy/plane-mcp:latest"
      ],
      "env": {
        "PLANE_API_KEY": "your-plane-api-key",
        "PLANE_BASE_URL": "https://plane.example.com",
        "PLANE_WORKSPACE_SLUG": "your-workspace-slug",
        "PLANE_MCP_PROFILE": "full"
      }
    }
  }
}
```

The Claude Desktop config file lives at:

- **macOS:** `~/Library/Application Support/Claude/claude_desktop_config.json`
- **Windows:** `%APPDATA%\Claude\claude_desktop_config.json`

Claude Desktop has no official Linux build — on Linux, use Claude Code or another MCP client that reads the same `mcpServers` schema. After editing the config, restart your client and call the `ping` tool to verify the connection.

## Configuration

All configuration is supplied via environment variables.

| Variable | Required | Default | Description |
|---|:---:|---|---|
| `PLANE_API_KEY` | ✅ | — | Personal Access Token for the Plane API. |
| `PLANE_BASE_URL` | ✅ | — | Base URL of your Plane instance (e.g. `https://plane.example.com`, or your self-hosted URL). |
| `PLANE_WORKSPACE_SLUG` | ✅ | — | The slug of the workspace to operate in. |
| `PLANE_MCP_PROFILE` | | `full` | Tool scoping profile: `worker`, `planner`, `reviewer`, or `full`. See [Tool Scoping Profiles](#tool-scoping-profiles). |
| `PLANE_MCP_TOOLS` | | — | Comma-separated explicit tool allowlist (e.g. `find_my_work,get_work_item`). When set, it **overrides** `PLANE_MCP_PROFILE` and only the listed tools are registered. |
| `CF_ACCESS_CLIENT_ID` | | — | Cloudflare Access service-token client ID, for routing to private/tunneled Plane deployments. |
| `CF_ACCESS_CLIENT_SECRET` | | — | Cloudflare Access service-token client secret. |

## Tool Scoping Profiles

The active profile determines which tools are exposed to the agent. `planner` and `full` currently expose the same complete tool set; `worker` and `reviewer` are deliberately restricted.

| Profile | Intended for | Surface |
|---|---|---|
| `worker` | Implementation agents | Read work items + report progress on their own tasks. No cross-project listing/search, no CRUD, relations, or hierarchy tools. |
| `reviewer` | Review agents | Read-only access plus comment-back. Can list/inspect items and comments, but cannot create, update, or transition work. |
| `planner` | Planning agents | The full tool set, including create/update, assignees, relations, parent/child hierarchy, and cross-project moves. |
| `full` | Unrestricted | Every tool (same surface as `planner`). |

For finer control than a profile gives, set `PLANE_MCP_TOOLS` to an exact comma-separated list of tool names.

## Tools Reference

A `ping` tool is always registered (connection check), regardless of profile. The remaining tools are gated as follows.

### Read & query

| Tool | Description | `worker` | `reviewer` | `planner` | `full` |
|---|---|:---:|:---:|:---:|:---:|
| `find_my_work` | List work items assigned to the current user, optionally filtered by project/state group. | ✅ | ✅ | ✅ | ✅ |
| `get_work_item` | Retrieve a single work item by identifier (e.g. `PROJ-123`). | ✅ | ✅ | ✅ | ✅ |
| `list_projects` | List all projects (identifier, name, id). | ✅ | ✅ | ✅ | ✅ |
| `list_labels` | List all labels in a project. | ✅ | ✅ | ✅ | ✅ |
| `list_states` | List all states in a project. | ✅ | ✅ | ✅ | ✅ |
| `list_work_items` | List work items in a project with optional filters. | | ✅ | ✅ | ✅ |
| `search_work_items` | Search work items across the workspace by text query. | | | ✅ | ✅ |
| `list_comments` | List all comments on a work item (HTML → Markdown). | | ✅ | ✅ | ✅ |
| `get_last_comment` | Get the most recently created comment on a work item. | | ✅ | ✅ | ✅ |
| `list_relations` | List all relations for a work item, grouped by type. | | | ✅ | ✅ |
| `list_children` | List a work item's child sub-issues. | | | ✅ | ✅ |

### Progress & comments

| Tool | Description | `worker` | `reviewer` | `planner` | `full` |
|---|---|:---:|:---:|:---:|:---:|
| `add_comment` | Add a Markdown comment to a work item. | ✅ | ✅ | ✅ | ✅ |
| `report_progress` | Post a progress comment and optionally transition state. | ✅ | | ✅ | ✅ |
| `submit_for_review` | Attach a PR link, comment, and move the item to `In Review`. | ✅ | | ✅ | ✅ |

### Labels & assignees

| Tool | Description | `worker` | `reviewer` | `planner` | `full` |
|---|---|:---:|:---:|:---:|:---:|
| `add_label` | Attach a label (by name or id) to a work item. | ✅ | | ✅ | ✅ |
| `remove_label` | Detach a label (by name or id) from a work item. | ✅ | | ✅ | ✅ |
| `assign_work_item` | Set / add / remove assignees by name, email, or id. | | | ✅ | ✅ |

### Create, update & hierarchy

| Tool | Description | `worker` | `reviewer` | `planner` | `full` |
|---|---|:---:|:---:|:---:|:---:|
| `create_task` | Create a new work item (Markdown description, labels, assignees, module). | | | ✅ | ✅ |
| `update_work_item` | Update a work item's name, description, priority, or state. | | | ✅ | ✅ |
| `set_relation` | Create a relation between two work items. | | | ✅ | ✅ |
| `remove_relation` | ⚠️ Not functional — the Plane API exposes no relation-removal endpoint for API-key auth; remove relations via the Plane web UI. | | | ✅ | ✅ |
| `set_parent` | Set a work item's parent. | | | ✅ | ✅ |
| `clear_parent` | Remove a work item's parent reference. | | | ✅ | ✅ |
| `move_work_item` | Move a work item to another project (copies fields; optionally deletes the original). | | | ✅ | ✅ |

## Building from Source

Requires Go 1.26+.

```bash
git clone https://github.com/ItsJennyFiggy/plane-mcp.git
cd plane-mcp

# Install dependencies
go mod download

# Run the test suite (race detector + coverage)
go test -v -race -cover ./...

# Build the binary
go build -o plane-mcp ./cmd/plane-mcp

# Or run directly with your environment configured
PLANE_API_KEY=... PLANE_BASE_URL=... PLANE_WORKSPACE_SLUG=... go run ./cmd/plane-mcp
```

## Contributing

Contributions are welcome. If you are working in this repository (human or AI agent):

1. Review the rules in `.agents/rules/` before making changes — especially `.agents/rules/git_safety.md` (secrets prevention) and `.agents/rules/testing_standards.md` (TDD and coverage gates).
2. Develop on a feature branch and open a pull request; do not push directly to protected branches.
3. Ensure `go test -race -cover ./...` passes and coverage gates are met before requesting review.
4. Follow the branching and PR lifecycle described in `.agents/workflows/git-workflow.md`.

## Security

Never commit credentials. `PLANE_API_KEY` and Cloudflare Access secrets should be supplied via environment variables or your MCP client's `env` block, not checked into source.

If you discover a security vulnerability, please report it privately via [GitHub Security Advisories](https://github.com/ItsJennyFiggy/plane-mcp/security/advisories/new) rather than opening a public issue.

## License

Released under the [MIT License](LICENSE).

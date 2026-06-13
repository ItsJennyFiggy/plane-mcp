# template-go

A scaffolding template for Go applications in the `ItsJennyFiggy` platform. It features a lightweight, standard-library-only `net/http` server with graceful shutdown, structured health checks, static distroless container builds, and fully configured CI/CD.

---

## Features

- **Standard Library Only**: No external router dependencies (e.g. Gin, Echo) keeping the baseline minimal and secure.
- **Production-Ready Server**: Exposes `/healthz` on port `8080` (or `PORT` env var) with configured read, write, and idle timeouts.
- **Graceful Shutdown**: Automatically catches system termination signals (`SIGINT`, `SIGTERM`) to finish active requests before exiting.
- **Multi-Stage Distroless Builds**: Compiles Go statically on `golang:1.26-bookworm` and packages it into `gcr.io/distroless/static-debian12:nonroot` for a minimal, read-only runtime environment.
- **Dual CI/CD Release Workflows**: Fully configured for publishing multi-arch images to GHCR (homelab) or AWS ECR (cloud) via OIDC.

---

## Repository Structure

```
├── .agents/
│   ├── rules/                  # Shared agent safety, testing, and dependency rules
│   │   ├── dependency_management.md
│   │   ├── environment_bootstrapping.md
│   │   ├── subagent_orchestration.md
│   │   └── testing_standards.md
│   ├── skills/
│   │   └── dependency-auditor/ # Dependency audit skill and license checker
│   └── workflows/
│       └── bootstrap.md        # Local environment bootstrapping workflow
├── cmd/
│   └── app/
│       ├── main.go             # Application entrypoint
│       └── main_test.go        # HTTP server unit tests
├── internal/
│   └── .gitkeep                # Private package library directory
├── .github/
│   └── workflows/
│       ├── ci.yml              # Go CI pipeline (vet, test, build)
│       ├── release.yml         # release-please orchestrator
│       ├── release-ghcr.yml    # Build & push to GHCR on release tag
│       └── release-ecr.yml     # Build & push to ECR on release tag
├── docs/
│   └── templates/
│       └── PROJECT_PLANNING.md # Project scoping template
├── .editorconfig               # Indentation and line-ending standards
├── release-please-config.json  # release-please config
├── .release-please-manifest.json # release-please manifest
├── CLAUDE.md                   # Agent rules index for this repo
├── Dockerfile                  # Multi-stage distroless build
├── go.mod                      # Go module configuration
├── LICENSE                     # MIT License
├── README.md                   # This file
└── README.template.md          # Blank README template for child repos scaffolded from this one
```

---

## Creating a New Go Service from This Template

1. Create a new repository using this template on GitHub.
2. Choose your release registry:
   - For **GHCR** (default for homelab): keep `.github/workflows/release-ghcr.yml` and delete `.github/workflows/release-ecr.yml`.
   - For **AWS ECR** (cloud): keep `.github/workflows/release-ecr.yml` and delete `.github/workflows/release-ghcr.yml`. Customize the `ecr_repository` name inside the file.
3. Rename `README.template.md` to `README.md` (or rewrite `README.md` to describe your service).
4. Run `go mod edit -module github.com/ItsJennyFiggy/<your-new-repo>` to rename the module path.
5. Follow `.agents/workflows/bootstrap.md` for local environment setup.

---

## Local Development & Verification

### Running the Server

```bash
go run ./cmd/app
```

### Running Tests

To run the local unit test suite and audit coverage:

```bash
go test -v -race -cover ./...
```

---

## Agent Guidelines

If you are an AI coding agent working in this repository:

1. Read `.agents/rules/` before making any changes.
2. Follow `.agents/rules/git_safety.md` strictly — never stage secrets or `.env` files.
3. Run the full test suite and verify coverage gates before opening a PR (see `.agents/rules/testing_standards.md`).
4. Follow the branch and PR lifecycle in `.agents/workflows/git-workflow.md`.

---

## Licensing

Licensed under the MIT License. See [LICENSE](LICENSE).

---
description: Verify host toolchains, install dependencies, configure local environment variables, run migrations, seed data, and run tests.
---

# Workflow: Local Environment Bootstrapping

This workflow maps to the `/bootstrap` command. Run this workflow immediately when bootstrapping a newly cloned repository or updating a branch with major upstream shifts.

---

## Step 1: Prerequisite Verification

Check if the required runtime environments (e.g., Node, Python, Go, Rust, or Docker) and package managers are installed on the host system. Run standard version checks (e.g., `<runtime> --version` or `<package-manager> --version`) to verify availability.

---

## Step 2: Install Runtime Dependencies

Install project packages localized within the workspace directory using the project's package manager:
```bash
# Run the appropriate dependency installation command for this repository
# (e.g., npm ci, pnpm install, uv sync, go mod download, cargo build)
<dependency-install-command>
```

---

## Step 3: Environment Configuration Setup

1.  Check if a local environment configuration file (e.g., `.env` or `config.json`) exists.
2.  If it does **NOT** exist, copy it from the workspace template:
    ```bash
    # (e.g., cp .env.template .env)
    <env-template-copy-command>
    ```
3.  Audit the newly created environment file to ensure no dummy values block local boot. If keys are missing, ask the user or check reference documentation.

---

## Step 4: Database & Cache Initialization

If the repository depends on local databases, cache engines, or storage layers:
1.  **Start Services**: Launch local development containers or mock services (e.g., using `docker compose up -d` or localized script commands).
2.  **Database Migrations**: Execute the repository's migration pipeline to sync the schema structure:
    ```bash
    # Run the database migration command for this repository
    # (e.g., prisma migrate dev, alembic upgrade head, goose up)
    <db-migration-command>
    ```
3.  **Mock Data Seeding**: Run the project's seeding scripts or commands to load initial testing datasets if available.

---

## Step 5: Boot Verification Suite

Run the full local verification harness to ensure the environment is fully bootable and passing checks:
1.  **Run Test Suite**:
    ```bash
    # Run the test runner for this repository
    # (e.g., npm test, pytest, go test, cargo test)
    <test-run-command>
    ```
2.  **Log Verification**: Confirm there are no unhandled exceptions, boot loops, or missing library failures in the outputs.

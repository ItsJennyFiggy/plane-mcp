# {{PROJECT_NAME}}

{{PROJECT_DESCRIPTION}}

---

## 🎯 Features & Scope

Briefly summarize the features and capabilities of this project.

*   **Feature A**: Description
*   **Feature B**: Description

---

## 🛠️ Tech Stack & Architecture

*   **Runtime/Language**: [e.g., Python 3.11, Node.js 20, Go 1.21]
*   **Frameworks/Libraries**: [e.g., FastAPI, Express, Gin]
*   **Database/Storage**: [e.g., PostgreSQL, Redis, SQLite]
*   **Infrastructure**: Deployed on the platform via automated GitOps pipelines.

---

## 🚀 Getting Started

### Prerequisites

Detail any dependencies required to run the project locally (e.g., Docker, Python/Node, package managers).

### Local Setup

1. Clone the repository and navigate to the directory.
2. Initialize configuration / environment variables:
   ```bash
   cp .env.example .env
   ```
3. Install dependencies:
   ```bash
   # Add dependency installation command here
   ```
4. Run the application locally:
   ```bash
   # Add command to run the local dev server
   ```

### Running Tests

To run the local test suite and audit code coverage (refer to `.agents/rules/testing_standards.md` for standards):

```bash
# Add test execution command here
```

---

## 🤖 AI Agent Guidelines

If you are an AI coding agent working in this repository:
1. Always audit the local `.agents/rules/` directory before making changes.
2. Adhere strictly to `.agents/rules/git_safety.md` to prevent secrets exposure.
3. Prior to presenting work or opening a PR, ensure all test suites pass and coverage gates are met (refer to `.agents/rules/testing_standards.md`).
4. Follow the standard branching and PR lifecycle detailed in `.agents/workflows/git-workflow.md`.

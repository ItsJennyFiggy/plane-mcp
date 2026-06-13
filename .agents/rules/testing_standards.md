---
trigger: model_decision
description: Enforces test-driven development (TDD), mocking boundaries, and coverage metrics across all application code files.
---

# Agent Rule: Testing Standards and Verification

This rule governs the development and validation of code within this repository. The agent MUST follow this rule without exception whenever modifying files.

---

## 1. Test-Driven Development (TDD) Mandate
Before writing or modifying any implementation code, the agent MUST write tests.
*   **For Bug Fixes**: Write a regression test that fails under the current implementation. Fix the code to make it pass.
*   **For Features**: Outline the input/output boundaries of the feature in unit tests before writing the business logic.
*   **Exclusion**: Boilerplate files (e.g., config mappings, CLI declarations) do not require TDD but still require coverage.

---

## 2. Test Isolation & Mocking Guidelines
*   **Unit Tests**: Must run in memory and be isolated from the network, disk, and databases.
    *   Use mocks and stubs for all external APIs and database calls.
    *   **Strict Socket/Network Blocking**: Unit tests MUST run with network access blocked. Enable socket blocking flags or libraries in the test runner configuration to guarantee no external API requests are made during unit test runs.
    *   Unit tests must execute in under 10 seconds.
*   **Integration Tests**: Target interactions between modules (e.g., database writes, REST endpoint requests).
    *   Use local mock containers or SQLite in-memory databases.
    *   **Self-Contained Database Fallbacks**: If integration tests require an external service (e.g., databases, caches) and it is not running locally, the test harness should dynamically fall back to localized databases (such as SQLite in-memory) or mock containers to prevent test run failures.
    *   Keep integration tests in a separate directory (e.g., `tests/integration/` or `__tests__/integration/`) from unit tests.

---

## 3. Coverage Gates & Quality Metrics
All code changes must satisfy the following criteria:
*   **Coverage Minimums**: 
    *   New features/modules must have **85%** statement coverage.
    *   Overall repository test coverage must not decrease as a result of a commit.
*   **Safety Assertions**: Every test must contain explicit assertions (e.g., no raw try-except blocks wrapping tests without assertions).
*   **AAA Pattern (Arrange-Act-Assert)**: Structure tests cleanly into Arrange (setup data), Act (execute system under test), and Assert (verify outputs) phases to maximize readability.
*   **Flaky Test Protocol**: If a test fails inconsistently due to timing, state leakage, or external conditions, the agent MUST NOT disable, delete, or modify assertions to mask the failure. The agent must isolate the test, attempt a deterministic fix, and flag the issue clearly to the user.

---

## 4. Pre-Commit / Pre-PR Verification Checklist
Before staging code or presenting a task as complete to the user, the agent MUST:
1.  Locally run the test suite using the project's configured test runner command.
2.  Locally run code coverage and inspect metrics to ensure the coverage gate is met.
3.  Fix all failing tests. Under no circumstances may the agent disable, delete, or comment out failing tests to pass checks.
---

## 5. Failure Protocol
If a test fails during development:
1.  Analyze the execution trace.
2.  Do NOT modify or delete the test if it represents valid business expectations.
3.  Refactor the implementation code until the test passes.
4.  If the test itself was outdated or incorrect, document the reason for updating the test expectations in the commit message or PR description.

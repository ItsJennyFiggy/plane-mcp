---
name: dependency-auditor
description: Audits packages for freshness, security vulnerabilities, and license compliance. Use when adding, upgrading, or auditing third-party packages.
---

# Skill: Dependency Auditor

Use this skill to scan codebase dependencies for version freshness, security advisories, and license compliance.

---

## 1. Freshness Audit

Identify packages that are outdated or lagging behind stable upstream releases to prevent version drift:
*   Locate the active package configuration file (e.g., manifest or lockfile).
*   Run the package manager's command to detect outdated packages (e.g., run `outdated` or dependency list checks).
*   Compare current versions against the latest stable registry releases.

---

## 2. Licensing Audit

Ensure no copyleft or highly restrictive dependencies (e.g., GPL, AGPL) are introduced into the codebase:
1.  **Extract License Metadata**: Use the active toolchain's package metadata tools or packaged license scripts (such as the repository's `check_licenses` scripts) to dump dependency license info.
2.  **Verify Compliance**:
    *   **Allowed**: Permissive licenses (e.g., MIT, Apache 2.0, BSD-2-Clause, BSD-3-Clause, ISC).
    *   **Requires User Review**: Weak copyleft licenses (e.g., LGPL, MPL).
    *   **Prohibited**: Restrictive copyleft licenses (e.g., GPL v2/v3, AGPL) or non-commercial licenses (e.g., CC-BY-NC).

---

## 3. Security Advisory Scan

Scan package manifests and lockfiles against known vulnerability databases:
*   Execute the package manager's built-in audit/security scanning utilities if supported by the toolchain.
*   If custom static security check CLI tools are pre-configured in the workspace, run them locally before staging changes.
*   Verify that no high-severity security advisories affect any newly added dependencies.

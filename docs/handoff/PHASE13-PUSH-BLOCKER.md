# Phase 13 workflow activation blocker

Date: 2026-07-29

The Phase 13 runtime, tests, threat model, deterministic SBOM, license report, and local quality gates are ready. The desired hardened Product CI definition is preserved at [`PRODUCT-CI-HARDENING.pending.yml`](PRODUCT-CI-HARDENING.pending.yml).

GitHub rejected the first push because the temporary Personal Access Token is not permitted to create or update `.github/workflows/product-ci.yml` without GitHub's workflow-management permission. No force-push was attempted. The active workflow was therefore left unchanged rather than weakening or bypassing GitHub's control.

To activate later:

1. Review the pending workflow.
2. Use a fine-grained credential with explicit Workflows write permission, or an approved GitHub App.
3. Copy it to `.github/workflows/product-ci.yml` on the same branch.
4. Commit normally (no force-push) and verify staticcheck, govulncheck, SBOM, license, checksum, and clean-tree steps in GitHub Actions.

This is an authorization gate, not a software-test failure. The same commands passed locally before the Phase 13 commit.

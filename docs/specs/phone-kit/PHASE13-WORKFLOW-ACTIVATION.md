# Phase 13 hardened workflow activation

Date: 2026-07-29
Status: **RESOLVED**

GitHub initially rejected an update to `.github/workflows/product-ci.yml` because the first temporary credential lacked Workflows write permission. No force-push, bypass, or history rewrite was attempted; the desired workflow was preserved for review.

An approved replacement credential with the required permission was then used to activate the reviewed workflow normally:

- Commit: `9376d77d2e1dfa387b5129e88446b441e8b48eef`
- Push CI: https://github.com/mohamedmasoud3030-tech/mosaid/actions/runs/30452139545
- Draft PR CI: https://github.com/mohamedmasoud3030-tech/mosaid/actions/runs/30452138991

Both runs passed the expanded gates: formatting, module checksum verification, unit and race tests, vet, staticcheck, govulncheck, Linux/Android builds, binary checksums, secret scan, deterministic CycloneDX SBOM comparison, license comparison, clean-tree verification, and artifact upload.

The active source of truth is now `.github/workflows/product-ci.yml`; the temporary pending copy was removed.

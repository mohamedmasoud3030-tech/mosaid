# CI activation package (pending owner credential)

These three files are the exact workflow changes that GitHub rejected on
2026-08-19 because the session's App credential lacks the `workflows`
permission (same mechanism as Phase 13). When an owner-approved credential
with Workflows write is available through Arena's secure channel, activate
as follows — all steps are reversible.

## Why

The Go vulnerability database gained standard-library entries affecting
Go 1.25.12, fixed in 1.25.13 (verified locally: 1.25.12 → govulncheck exit 3
with 4 stdlib findings; 1.25.13 → no findings). Both CI workflows pin
Go 1.25.12 (and `actions/setup-go` pins `GOTOOLCHAIN=local`, so the
`toolchain` directive in go.mod cannot switch CI).

## Activation procedure

1. With the owner-approved credential active, copy the three files into
   `.github/workflows/`:

   ```sh
   cp docs/handoff/activation/product-ci.yml   .github/workflows/product-ci.yml
   cp docs/handoff/activation/phase0-ci.yml    .github/workflows/phase0-ci.yml
   cp docs/handoff/activation/measure-phase0.yml .github/workflows/measure-phase0.yml
   git add .github/workflows && git commit -m "ci: Go 1.25.13 gates + phone-kit job + phase0 re-pin measurement"
   git push origin arena/01a01769-mosaid
   ```

   Expected after this push: `Mosaid product CI` green (including the new
   `phone-kit` job); `Phase 0 preservation CI` red at the pinned-hash
   comparison (the pins are still the 1.25.12 values — the next step
   re-pins them).

2. Dispatch the measurement workflow:

   ```sh
   gh workflow run measure-phase0.yml --ref arena/01a01769-mosaid
   ```

   It rebuilds the Phase 0 binary/kit with Go 1.25.13 on the real proxy,
   re-measures govulncheck, SBOM, licenses, and toolchain archive hash, and
   commits the updated pins (`manifests/*`, `release/*`,
   `evidence/*`, `expected-govulncheck.txt`) back to the branch.

3. `git fetch && git pull` and review the re-pin commit diff (hashes,
   `remaining_findings`, SBOM, license report).

4. Update the human-readable docs to the new pins and add the re-pin note:
   - `phase0-android-runtime/README.md` (Go 1.25.13, binary/kit hashes)
   - new `phase0-android-runtime/docs/REPIN-2026-08-19.md` (why + evidence)
   - `docs/handoff/PHASE14-PHONE-KIT.md` (CI limitations resolved)
   - root `README.md` if it references the Phase 0 identity.
   Push → both workflows run green on the PR.

5. Remove the temporary workflow and re-verify:

   ```sh
   rm .github/workflows/measure-phase0.yml
   git commit -am "ci: remove temporary phase0 re-pin measurement workflow" && git push
   ```

   `measure-phase0.yml` remains archived here for reproducibility.

## Safety notes

- The measurement workflow commits with the runner's `GITHUB_TOKEN`
  (`contents: write`). It does not force-push and only adds/modifies
  Phase 0 manifest/release/evidence files.
- `DELIVERABLES.sha256` is a historical point-in-time record of the
  original Phase 0 delivery and is intentionally not regenerated.
- If the measured `remaining_findings` contains anything beyond the
  documented `GO-2026-5932` openpgp exception, stop and review before
  finalizing the pin.

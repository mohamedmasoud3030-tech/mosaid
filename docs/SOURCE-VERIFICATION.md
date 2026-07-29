# Phase 0 — Source and release verification

Cut-off: 2026-07-29 (Asia/Muscat)

## Pinned source identity

| Item | Value |
|---|---|
| Repository | `https://github.com/sipeed/picoclaw` |
| Tag name | `v0.3.1` |
| Annotated tag object | `9fba4cec050cbfe3d73dfcfe015d7960447b9c7f` |
| Peeled commit | `2cf030d2fd3b871d7ec17e3be34c24688aac76da` |
| Commit tree | `79530d185c4c5eb30719fd45cf323217d2a9f5c5` |
| Commit time | `2026-06-30T09:42:07Z` |
| Original `go.mod` directive | `go 1.25.11` |
| License | MIT |

The annotated tag is **unsigned**. GitHub reports the peeled commit signature as valid, but this does not make the tag immutable. The release API also reports `immutable: false`. The qualification workflow therefore checks all three of tag object SHA, peeled commit SHA, and tree SHA before building.

Evidence is retained in `evidence/upstream/tag-ref.json`, `tag-object.json`, `commit.json`, and `release.json`.

## Official release workflow

The official `release.yml`:

1. Verifies that the named tag exists.
2. Checks out that tag.
3. Uses `actions/setup-go` with `go-version-file: go.mod`.
4. Runs GoReleaser with `INCLUDE_ANDROID_BUNDLE=true`.
5. GoReleaser invokes `make build-android-bundle`.

The Makefile builds the core with:

```text
GOOS=android GOARCH=arm64 go build -tags stdjson ... ./cmd/picoclaw
```

It stages the result as `arm64-v8a/libpicoclaw.so`; despite the `.so` suffix it is an executable Android PIE, not a shared library. A second executable, `libpicoclaw-web.so`, contains the launcher/web component.

The embedded Go build information in the official core records:

- Go `1.25.11`.
- VCS revision `2cf030d2fd3b871d7ec17e3be34c24688aac76da`.
- VCS modified `false`.
- Android/ARM64 and `CGO_ENABLED=0`.

This is strong evidence that the release core came from the pinned commit. It is not independent signed provenance.

## Official Android bundle inspection

`picoclaw-android-universal.zip` contains exactly:

```text
arm64-v8a/libpicoclaw.so
arm64-v8a/libpicoclaw-web.so
```

There is no APK, installer, shell script, second architecture, or signature file inside it.

| File | SHA-256 | Type |
|---|---|---|
| Android ZIP | `fdfc0dbf52af7dfa82022629c902796fb479cc62a483b86cb86d2919b8f191ee` | ZIP |
| `libpicoclaw.so` | `7a1ab761530422e778b1807923fc4c3eaeee007e94f23559dfeaca2dfd4cb789` | AArch64 Android PIE, `/system/bin/linker64` |
| `libpicoclaw-web.so` | `9794b4f78e0dd250d031470d6b7d6921496cd5755574425aea99cbf87c711ce0` | AArch64 Android PIE, `/system/bin/linker64` |

The GitHub release asset API publishes the Android ZIP digest and the locally calculated SHA-256 matches it. However, `picoclaw_0.3.1_checksums.txt` does **not** list the Android ZIP. There are no Sigstore, cosign, minisign, or SLSA provenance assets in the release.

A notable release-quality anomaly is that the official Android core embeds `Version=nightly` even though it was attached to `v0.3.1`.

## Linux ARM64 artifact

The official Linux archive SHA-256 is:

```text
e78ab0de05d000698ff8b32458e71cb3134e52e9eaf540e3323a72e9914de060
```

It is present in the official checksums file. It contains `picoclaw`, `picoclaw-launcher`, README, and LICENSE. The core is a statically linked Linux AArch64 executable.

It is not selected for Phase 0 because:

- `GOOS=linux` is not the same target as Android/Bionic.
- It includes an unnecessary launcher.
- Earlier Termux reports documented CA/DNS friction with the Linux artifact.
- A native `GOOS=android` core has the correct `/system/bin/linker64` interpreter.

## Independent rebuild comparison

An unmodified source rebuild used Go `1.25.11`, the exact official embedded ldflags, and the pinned source. It did **not** reproduce the official core byte-for-byte:

```text
official:     7a1ab761530422e778b1807923fc4c3eaeee007e94f23559dfeaca2dfd4cb789
rebuild:      1c45ef862334d36d342075a6f28b57ed5bce16e09c1a4598c62a77a38dafa6cf
rebuild+git:  ad2f1991ea93c51bf721c55c8815bd2a422272d06c2cb8169b624fe0677901a6
```

The differences are consistent with non-reproducible build environment details: VCS stamping, source path, Go build ID/action graph, and runner environment. Therefore no claim is made that the upstream release is reproducible.

## Qualification build decision

The official artifact is **not used on the phone** because its full default surface contains tools/channels outside Phase 0 and because it was built with versions now flagged by the current Go vulnerability database.

The qualification binary is built from the exact commit plus two isolated patches:

1. `phase0-qualification.patch`
   - one numeric Telegram owner;
   - private chat only;
   - `/status` and bounded `/echo`;
   - safe message-ID/latency instrumentation;
   - disables the out-of-scope Kagi client, whose generated repository has no license file.
2. `phase0-security-overrides.patch`
   - Go dependency security floors only.

The build uses Go `1.25.12`, `GOOS=android`, `GOARCH=arm64`, `CGO_ENABLED=0`, `-trimpath`, no VCS auto-stamping, and an empty Go build ID. Two consecutive clean output builds produced the same SHA-256:

```text
b68746ddeeb341c291da5f93f59f857cdd892d8fe76940367604a2ec1c729a4f
```

This reproducibility claim applies only to the controlled qualification build in the recorded environment—not to upstream `v0.3.1` generally.

## Dependency and vulnerability findings

- 93 Go modules remain linked. This is much larger than the future target and confirms that product-level pruning is still required.
- The CycloneDX SBOM is `manifests/sbom.cdx.json`.
- `license-report.tsv` has no missing/unknown license among linked modules after disabling Kagi.
- `govulncheck` leaves one module-level finding: `GO-2026-5932` for the unmaintained `x/crypto/openpgp` package.
- `go list -deps -tags stdjson ./cmd/picoclaw` confirms that `golang.org/x/crypto/openpgp` is not imported. The scanner exception is documented for qualification only; it is not a claim of zero risk.

## Conclusion of source verification

- **Official artifact provenance:** adequate for inspection, insufficient for a security-sensitive custom deployment.
- **Official Android artifact:** correct architecture but too broad for this PoC.
- **Linux ARM64 artifact:** fallback only, not preferred.
- **Controlled Android build from pinned commit:** selected for phone qualification.

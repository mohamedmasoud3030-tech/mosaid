# Restore the original local Phase 0 history

The archival GitHub prerelease attaches:

- `phase0-local-all-refs.bundle`
- `phase0-working-tree-complete.tar.gz`
- `phase0-preservation.sha256`

The bundle preserves the original independent local repository refs before Mosaid restructuring:

```text
branch: phase0/android-runtime-qualification
HEAD:   91e6758475fd91cb92b9585f260642704586ce2e
tag:    phase0-harness-v1
```

Verify and inspect:

```bash
sha256sum -c phase0-preservation.sha256
git bundle verify phase0-local-all-refs.bundle
git bundle list-heads phase0-local-all-refs.bundle
```

Clone the original history:

```bash
git clone phase0-local-all-refs.bundle phase0-original
cd phase0-original
git switch phase0/android-runtime-qualification
```

Inspect the complete working-tree archive without extracting:

```bash
tar -tzf phase0-working-tree-complete.tar.gz
```

Extract separately if ignored evidence/release outputs are needed:

```bash
mkdir phase0-working-tree
mkdir phase0-working-tree/extracted
# Run from the dedicated directory to avoid overwriting a repository.
tar -xzf ../phase0-working-tree-complete.tar.gz -C extracted
```

The working-tree archive excludes `.git`, `.security.yml`, real `.env` files, private phone results, and credential files. It includes important ignored Phase 0 evidence and release outputs.

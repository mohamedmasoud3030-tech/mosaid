# Product dependency review

Checkpoint: 2026-07-29.

The product intentionally keeps two direct Go module dependencies:

1. `modernc.org/sqlite v1.53.0` — pure-Go SQLite needed for Android cross-compilation without CGO.
2. `github.com/modelcontextprotocol/go-sdk v1.7.0` — official MCP Go SDK, pinned for explicit stdio and Streamable HTTP clients.

Transitive modules are captured in the deterministic CycloneDX SBOM and license report. The hardening gate verifies module sums, runs source-reachability vulnerability analysis, rejects unknown or GPL/LGPL license classifications, and fails if regenerated evidence differs from the tracked files.

No dependency enables dynamic Go plugins, a persistent Node/Python runtime, browser automation, a local model, or a shell. The Phase 0 PicoClaw archive and its 93-module qualification binary are historical evidence and are not linked into the Mosaid product binary.

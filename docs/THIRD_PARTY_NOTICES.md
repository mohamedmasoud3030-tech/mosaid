# Third-party notices

Mosaid contains research material and a Phase 0 qualification derivative based on third-party open-source software. Original licenses and notices must remain with those components.

## PicoClaw

- Project: PicoClaw
- Repository: https://github.com/sipeed/picoclaw
- Version/tag: `v0.3.1`
- Tag object: `9fba4cec050cbfe3d73dfcfe015d7960447b9c7f`
- Commit: `2cf030d2fd3b871d7ec17e3be34c24688aac76da`
- License: MIT
- Original license evidence: `phase0-android-runtime/evidence/upstream/LICENSE`
- Copy included in the phone kit: `licenses/PicoClaw-LICENSE`

### Used or derived portions

The qualification patches are applied to the pinned PicoClaw source at build time:

- `phase0-android-runtime/patches/phase0-qualification.patch`
  - modifies `pkg/channels/telegram/telegram.go`;
  - adds `pkg/channels/telegram/phase0_qualification_test.go`;
  - modifies `pkg/tools/integration/web.go` to prevent linking an out-of-scope generated Kagi client with no discovered license file.
- `phase0-android-runtime/patches/phase0-security-overrides.patch`
  - modifies `go.mod` and `go.sum` only to raise security floors.

Nature of modification:

- exactly one numeric Telegram owner;
- private-chat-only qualification ingress;
- bounded `/echo` and non-sensitive `/status` diagnostics;
- safe message-ID/reply/latency instrumentation;
- qualification-only removal of Kagi client linkage;
- dependency security updates;
- reproducible Android ARM64 build metadata.

The resulting binary in `phase0-phone-kit.tar.gz` is a derivative of PicoClaw and is not represented as wholly original Mosaid code.

## Telego

- Project: Telego — Telegram Bot API library for Go
- Repository: https://github.com/mymmrac/telego
- Version linked by Phase 0: `v1.10.0`
- License: MIT

Telego is pulled transitively/directly by the pinned PicoClaw module and is recorded in the SBOM and license report.

## Model Context Protocol Go SDK

- Project: Model Context Protocol Go SDK
- Repository: https://github.com/modelcontextprotocol/go-sdk
- Version linked by Phase 0: `v1.6.1`
- License: project licensing transition covering Apache-2.0 and unrelicensed MIT contributions, as stated by its LICENSE.

MCP is disabled in Phase 0. The module remains linked through upstream PicoClaw's broad compile-time surface and is recorded for transparency.

## Other Go modules

The complete set actually linked into the qualification binary is recorded in:

- `phase0-android-runtime/sbom/sbom.cdx.json`
- `phase0-android-runtime/sbom/linked-modules.tsv`
- `phase0-android-runtime/sbom/license-report.tsv`

The best-effort license report includes MIT, Apache-2.0, BSD, ISC, MPL-2.0, and EPL-2.0/EDL-1.0 components. It is evidence, not legal advice. No missing/unknown linked license remained after the qualification build disabled the Kagi client linkage.

## Projects referenced as research only

Research and architecture documents discuss Hermes Agent, nanobot, OpenClaw, PydanticAI, LangGraph, Open Interpreter, OpenHands, Agent Zero, AutoGen, CrewAI, ZeroClaw, NullClaw, Moltis, and others. Their code was not merged into Mosaid. Links, architectural observations, and brief descriptions do not change their original licenses.

## Mosaid license boundary

Original Mosaid documentation, scripts, repository governance, and future original code are licensed under the root `LICENSE` unless explicitly stated otherwise. Third-party and derivative files retain the licenses listed here and in their own notices.

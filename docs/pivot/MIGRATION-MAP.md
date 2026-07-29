# Mosaid to Hermes Migration Map

Date: 2026-07-29

## Keep as Mosaid product assets

These are the differentiating parts of Mosaid and should be migrated or reimplemented above Hermes:

- Arabic and Egyptian-Arabic identity and interaction rules.
- Beginner-guided execution: explain, prepare, execute, verify, teach and deliver.
- Zero-cost operating policy and no-paid-fallback rule.
- Digital-work Work Packs.
- Portfolio Builder.
- Opportunity Engine.
- Client Work Manager.
- Approval points for proposals, prices, deadlines, publishing, delivery and external communication.
- Arabic planning, tool-selection, verification and prompt-injection benchmarks.
- Product-specific audit and evidence requirements.

## Reuse from PR #3 selectively

PR #3 is not merged, but the following concepts and fixtures are reusable after review:

- `docs/architecture/*` product and zero-cost documentation.
- Work Pack definitions and completeness rules.
- Arabic benchmark scenarios and expected behavior.
- Provider cost classifications and failure cases.
- Resume/idempotency test ideas.
- Tool allowlist/denylist tests.
- SQLite persistence design lessons.

Do not copy code mechanically. Port only what fits the Hermes extension and configuration interfaces.

## Replace with Hermes runtime capabilities

Do not continue maintaining parallel implementations of:

- general agent loop;
- Telegram gateway;
- generic provider router;
- generic memory engine;
- generic scheduler;
- generic MCP client;
- generic Skill loader;
- generic cloud/SSH execution backend;
- generic subagent orchestration.

## Mosaid extension layout

Target repository layout:

```text
mosaid/
├── product/
│   ├── identity/
│   ├── policies/
│   ├── prompts/
│   ├── skills/
│   ├── workpacks/
│   ├── benchmarks/
│   └── templates/
├── deploy/
│   └── hermes/
├── docs/
│   ├── pivot/
│   ├── product/
│   └── operations/
└── legacy-go-foundation/
    └── preservation-notes.md
```

The existing Go files stay in place until the Hermes deployment passes the pivot acceptance gate. Moving them into a legacy directory is a later explicit migration, not part of the first pivot commit.

## Work Pack migration order

1. Research and information gathering.
2. Content writing and editing.
3. Virtual assistant operations.
4. Spreadsheet and reporting work.
5. Coding assistance.
6. Social media operations.
7. Customer support.
8. Lead generation.
9. Ecommerce operations.
10. Basic websites and landing pages.
11. Job and freelance applications.

Each pack must contain:

- intake questions;
- scope boundaries;
- required tools;
- approval points;
- workflow steps;
- quality checks;
- delivery artifacts;
- beginner guidance;
- portfolio-safe redaction;
- deterministic tests or review fixtures.

## Safety migration rules

- Hermes Skills Hub content is never auto-enabled.
- Self-created Skills remain drafts until owner approval.
- Terminal access is restricted to a dedicated workspace user and path.
- External actions remain approval-bound.
- No model may approve its own action.
- No upstream update is installed automatically.
- No Oracle auth token, SSH private key, API key or Telegram token is committed.

## Completion definition

Migration is not complete when files merely exist. It is complete when a real owner-only Telegram request reaches Hermes on Oracle, loads Mosaid policy and identity, executes one approved Work Pack, persists the result across restart and returns evidence to the user.

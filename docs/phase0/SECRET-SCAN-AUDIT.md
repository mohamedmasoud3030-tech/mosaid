# Phase 0 preservation secret-scan audit

Date: 2026-07-29

## Scope

Scanned before push/release:

- complete Mosaid working tree;
- every commit in the Mosaid history to be pushed;
- original Phase 0 Git bundle by cloning it into a clean directory;
- complete working-tree preservation archive after extraction;
- Phase 0 phone kit after extraction;
- Android qualification binary as bytes/strings;
- tracked reports, logs/evidence, manifests, patches, configs, and examples.

Patterns included:

- GitHub fine-grained and classic tokens;
- Telegram bot tokens;
- OpenAI/OpenRouter/Anthropic-style keys;
- Authorization bearer/token headers;
- private-key PEM headers;
- repository `.env` and `.security.yml` paths.

## Result

**No real secret was found in the files, histories, or release assets selected for upload.**

No `.security.yml` or real `.env` file is present. `.env.example` files contain variable names only.

## Binary false positives reviewed

Raw binary scanning found three key-like static sequences:

1. One explicit upstream example/placeholder string.
2. One 23-byte, all-lowercase identifier-shaped sequence with separators, low character entropy, and no source literal match. It is far shorter and structurally unlike a real provider credential; its reviewed SHA-256 prefix is `54f6a0d6ed9b8436`.
3. One Anthropic-shaped sequence formed in the linked binary/string evidence without a token boundary. It was absent as an exact literal from the pinned PicoClaw source and all downloaded Go module source files. It is consistent with linker string concatenation/detection data, not a credential supplied during this work.

The values are intentionally not reproduced here. Their hashes, boundaries, entropy, source absence, and classification were reviewed locally without printing the values. The qualification build never received a Telegram/model/GitHub runtime secret.

## Preventive controls

- Root and Phase 0 `.gitignore` files block `.env`, `.security.yml`, private keys, phone results, diagnostics, logs, and credential files.
- CI runs `phase0-android-runtime/scripts/scan-secrets.sh`.
- The phone diagnostics collector refuses packaging if an exact active secret appears outside `.security.yml`.
- The GitHub credential used solely for repository handoff is staged outside the repository and deleted after transfer.

## Limitation

Pattern scanners cannot prove absence of every encoded or fragmented secret. The result combines pattern scanning, archive extraction, history inspection, binary review, and knowledge that no runtime credentials were introduced during Phase 0 construction.

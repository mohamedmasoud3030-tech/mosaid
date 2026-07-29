# Social-publishing skill

Use the official Meta Graph API only. `prepare` validates an existing image artifact and creates an idempotent local draft. `preview` displays the immutable account, asset, caption hash, and publish-time binding. `publish` is routed through the core Tool Registry and requires a non-expired, single-use approval bound to all of those values.

Media staging, container creation, bounded status polling, recovery, retry, idempotency, cleanup, and hash-chained audit are implemented behind interfaces. No browser automation, usernames/passwords, cookies, unofficial APIs, or automatic publishing are permitted. Tests use mocks only; real Meta credentials and an Instagram Professional account remain `PENDING_EXTERNAL_VALIDATION`.

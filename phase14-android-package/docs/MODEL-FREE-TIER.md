# Model selection: free-only (Google Gemini free tier)

## Decision

Use the **Google Gemini free tier through AI Studio** as the default model
provider. It is the only major provider with an ongoing, no-card, no-expiry
free tier and an officially documented OpenAI-compatible endpoint, which the
Mosaid runtime already speaks.

- Base URL: `https://generativelanguage.googleapis.com/v1beta/openai`
  (official documentation: https://ai.google.dev/gemini-api/docs/openai)
- Default model: `gemini-2.5-flash` (stable, free tier)
- API key: created at https://aistudio.google.com/apikey — no credit card.

## Free tier limits (2026 figures; check the AI Studio dashboard for current values)

| Model | Requests/min | Requests/day | Notes |
|---|---|---|---|
| gemini-2.5-flash (default) | ~10 | ~250 | stable, 1M context |
| gemini-2.5-flash-lite | ~15 | ~1,000 | lighter, higher limits |
| gemini-2.5-pro | ~5 | ~100 | stronger, lower limits |

Sources: https://www.aifreeapi.com/en/posts/gemini-api-free-tier-complete-guide
and https://www.apideck.com/blog/how-to-get-your-gemini-api-key — both
aggregate the limits Google publishes per-project in AI Studio. The runtime
mirrors the tightest limit in its Telegram flood control
(`messages_per_minute: 10`, `message_burst: 3`) so a quota burst fails
closed locally instead of hammering the API.

## Why no charge is possible

- The free tier has no linked billing account, so nothing can be charged.
- The runtime has no cost accounting (`security.Budget.UseCost` is never
  called), and `limits.max_cost_usd` is pinned to `0.01` — the lowest value
  config validation accepts — as a fail-closed tripwire for any future paid
  usage. `verify-config.sh` fails if that value changes.

## Privacy

On the free tier Google may use prompts and responses to improve its
products. Therefore: do not share passwords, keys, tokens, or other private
data in bot conversations. The phone kit is for a private, owner-only bot;
treat chats as visible to the model provider.

## Alternatives (same config, different values)

| Provider | Base URL | Notes |
|---|---|---|
| Groq | `https://api.groq.com/openai/v1` | free tier, OpenAI-compatible |
| Mistral | `https://api.mistral.ai/v1` | free tier, OpenAI-compatible |
| OpenRouter | `https://openrouter.ai/api/v1` | `:free` model variants |

If the Gemini endpoint is unreachable from the owner's network (regional
blocking), switch any of the three values — base URL, model, key — via the
installer prompts or by re-running `install-phone.sh`.

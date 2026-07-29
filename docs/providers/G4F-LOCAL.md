# G4F (GPT4Free) Local Provider — Setup Guide

**Date:** 2026-07-29  
**Status:** Accepted  
**Provider Tier:** `local_self_hosted`

---

## What is G4F?

G4F (GPT4Free) is an open-source Python project (`github.com/xtekky/gpt4free`) that converts free AI website interfaces into a local OpenAI-compatible API server. It provides access to GPT-4.1, Claude 4 Sonnet, Gemini 2.5 Pro, and others via Poe, Bing, DuckDuckGo, and You.com.

---

## ⚠️ Important Warnings

1. **ToS Violation Risk:** G4F uses reverse engineering to access AI services. This may violate the terms of service of some providers.
2. **Personal Use Only:** Do NOT use G4F in commercial production. Use it for testing, development, and personal projects only.
3. **Instability:** Available models change frequently as websites update their APIs.
4. **No Guarantees:** Any provider can block G4F at any time.

**Mosaid classifies G4F as `local_self_hosted` with `stability: low` and `use_when: emergency_only`.**

---

## Setup Steps

### 1. Install G4F

```bash
pip install -U g4f[all]
```

### 2. Start the Local Server

```bash
python -m g4f.api.run --port 8080 --debug
```

The server starts at `http://localhost:8080` and exposes:
- `POST http://localhost:8080/v1/chat/completions` (OpenAI-compatible)
- `GET http://localhost:8080/v1/models` (list available models)

### 3. Configure Mosaid

```yaml
providers:
  g4f_local:
    base_url: "http://localhost:8080"
    model_id: "gpt-4o"  # or claude-4-sonnet, gemini-2.5-pro, etc.
    timeout_seconds: 60
```

---

## Available Models (July 2026)

| Model ID | Via | Quality | Speed |
|---|---|---|---|
| `gpt-4o` | Bing/Copilot | High | Medium |
| `gpt-4.1` | Bing/Copilot | Very High | Medium |
| `claude-3.7-sonnet` | Poe | Very High | Medium |
| `claude-4-sonnet` | Poe | Excellent | Slow |
| `gemini-2.5-pro` | Bard | Very High | Medium |
| `grok-3` | X.com | High | Medium |
| `o3-mini` | Bing | High | Fast |

**Note:** Model availability changes frequently. Check the G4F catalog for current options.

---

## Configuration in Mosaid

G4F is configured as the **emergency fallback** — it's only used when all other providers fail:

```yaml
fallback_chain:
  planning_and_reasoning:
    - provider: kaggle_tunnel      # First choice
    - provider: groq_free          # Second choice
    - provider: google_ai_studio   # Third choice
    - provider: cerebras_free      # Fourth choice
    - provider: g4f_local          # Emergency only
    - provider: mlcllm_local       # Last resort
    - action: persist_and_stop     # Never pay
```

---

## Troubleshooting

| Issue | Solution |
|---|---|
| Server won't start | Check Python version (3.10+), try `pip install -U g4f` |
| All models return errors | Website APIs may have changed, update G4F |
| Slow responses | Try a different model or provider within G4F |
| Rate limited | Wait a few minutes, or switch to a different G4F provider |
| Connection refused | Make sure the server is running on the correct port |

---

## Health Check

G4F exposes a health endpoint:

```bash
curl http://localhost:8080/v1/models
```

If this returns a valid model list, the server is healthy.

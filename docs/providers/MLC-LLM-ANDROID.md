# MLC LLM Android Provider — Setup Guide

**Date:** 2026-07-29  
**Status:** Accepted  
**Provider Tier:** `local_self_hosted`

---

## What is MLC LLM?

MLC LLM runs LLM models directly on your Android device's GPU via WebGPU. It's 5x faster than CPU-only inference (e.g., in Termux) and works completely offline.

---

## Setup Options

### Option 1: MLC Chat App (Easiest)

1. Download from [github.com/mlc-ai/mlc-llm/releases](https://github.com/mlc-ai/mlc-llm/releases)
2. Install the APK on your Android device
3. Choose a model to download:
   - **Best Arabic (4GB RAM):** `Qwen3-4B-Instruct-q4f16_1-MLC`
   - **Best Speed (2GB RAM):** `SmolLM3-3B-Instruct-q4f16_1-MLC`
   - **Best Coding (4GB RAM):** `Phi-4-Mini-Instruct-q4f16_1-MLC`
4. The app runs an OpenAI-compatible server on `http://127.0.0.1:8081`

### Option 2: Kiwi Browser + WebLLM (Uses Phone GPU)

1. Install [Kiwi Browser](https://kiwibrowser.com/) (supports extensions)
2. Open [webllm.mlc.ai](https://webllm.mlc.ai) in Kiwi Browser
3. Select a model and let it download
4. The model runs in-browser via WebGPU, accessing your phone's Adreno GPU

### Option 3: MLC LLM Python Server in Termux

```bash
# In Termux:
pip install mlc-llm
python -m mlc_llm serve Qwen3-4B-Instruct-q4f16_1 --host 127.0.0.1 --port 8081
```

---

## Recommended Models for Android

| Model | RAM | Speed (Snapdragon 8 Gen 3) | Arabic Quality |
|---|---|---|---|
| Qwen3-4B-Instruct-q4f16_1-MLC | 3.5GB | ~20 tok/s | Excellent for size |
| SmolLM3-3B-Instruct-q4f16_1-MLC | 2.2GB | ~30 tok/s | Medium |
| Phi-4-Mini-Instruct-q4f16_1-MLC | 3GB | ~22 tok/s | Medium |
| Llama-4-Scout-4B-Instruct-q4f16_1-MLC | 3.2GB | ~18 tok/s | Good |

---

## Configure Mosaid

```yaml
providers:
  mlcllm_local:
    base_url: "http://127.0.0.1:8081"
    model_id: "Qwen3-4B-Instruct-q4f16_1-MLC"
    timeout_seconds: 30
```

---

## When is MLC Used?

MLC is the **last resort** — used only when:
1. All online providers have failed
2. The device has no internet
3. Kaggle session is expired
4. G4F is also unavailable

```yaml
fallback_chain:
  planning_and_reasoning:
    - provider: kaggle_tunnel
    - provider: groq_free
    - provider: google_ai_studio
    - provider: cerebras_free
    - provider: g4f_local
    - provider: mlcllm_local          # ← Last resort
    - action: persist_and_stop
```

---

## Limitations

- **Small models only:** 3-4B parameters (vs 70B+ on Kaggle)
- **Limited context:** 4K-8K tokens (vs 32K+ on Kaggle)
- **Slower:** ~20 tok/s on phone GPU vs ~50 tok/s on Kaggle T4
- **RAM intensive:** Requires 2-4GB free RAM
- **Heats up phone:** Sustained use generates heat

---

## Troubleshooting

| Issue | Solution |
|---|---|
| Model won't load | Check available RAM, close other apps |
| Very slow | Ensure WebGPU is enabled, not falling back to CPU |
| App crashes | Try a smaller model (3B instead of 4B) |
| Port conflict | Change port in MLC config |
| No GPU acceleration | Check if your phone supports Vulkan/WebGPU |

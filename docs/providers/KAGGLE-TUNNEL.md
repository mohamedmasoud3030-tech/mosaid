# Kaggle Tunnel Provider — Setup Guide

**Date:** 2026-07-29  
**Status:** Accepted  
**Provider Tier:** `local_self_hosted`

---

## What is Kaggle Tunnel?

Kaggle provides **30 hours/week of free GPU** (T4 dual = 32GB VRAM, or P100 = 16GB) with no credit card required. By running a vLLM server inside a Kaggle Notebook and exposing it via a Cloudflared tunnel, Mosaid gets access to 70B+ parameter models for free.

---

## Setup Steps

### 1. Create a Kaggle Account

- Go to [kaggle.com](https://www.kaggle.com)
- Sign up (no credit card needed)
- Verify your phone number (required for GPU access)

### 2. Create a New Notebook

- Go to [kaggle.com/code](https://www.kaggle.com/code)
- Click "New Notebook"
- In Settings → Accelerator, select **GPU T4 x2**

### 3. Upload the Tunnel Script

Save this as a cell in your notebook:

```python
# Cell 1: Install dependencies
import subprocess
subprocess.run(["pip", "install", "-q", "vllm", "cloudflared"], check=True)

# Cell 2: Start vLLM server
import subprocess, time
vllm_proc = subprocess.Popen([
    "python", "-m", "vllm.entrypoints.openai.api_server",
    "--model", "Qwen/Qwen3-72B-Instruct-GPTQ-Int4",
    "--host", "0.0.0.0",
    "--port", "8000",
    "--max-model-len", "32768",
    "--gpu-memory-utilization", "0.90",
    "--dtype", "float16"
])
print("Waiting for vLLM to start...")
time.sleep(90)
print("vLLM should be ready.")

# Cell 3: Open Cloudflared tunnel
tunnel_proc = subprocess.Popen([
    "cloudflared", "tunnel", "--url", "http://localhost:8000"
])
tunnel_proc.wait()
```

### 4. Copy the Tunnel URL

When the tunnel opens, you'll see output like:

```
+--------------------------------------------------------------------------------------------+
|  Your quick Tunnel has been created! Visit it at (it may take some time to be reachable):  |
|  https://xyz-abc-123.trycloudflare.com                                                     |
+--------------------------------------------------------------------------------------------+
```

Copy the `https://xyz-abc-123.trycloudflare.com` URL.

### 5. Configure Mosaid

Add to your Mosaid config (or `providers.yaml`):

```yaml
providers:
  kaggle_tunnel:
    base_url: "https://xyz-abc-123.trycloudflare.com"
    model_id: "Qwen/Qwen3-72B-Instruct-GPTQ-Int4"
    timeout_seconds: 120
```

---

## Important Notes

- **Session duration:** ~9 hours, then the notebook shuts down automatically
- **Restart:** You need to re-run the notebook and update the tunnel URL
- **Multiple accounts:** You can use multiple Kaggle accounts for more hours
- **Model choice:** You can change the model in the vLLM command
- **No API key needed:** The tunnel is public (anyone with the URL can use it, but the URL changes each session)

---

## Alternative Models on Kaggle

| Model | VRAM | Arabic | Speed | Recommended For |
|---|---|---|---|---|
| Qwen/Qwen3-72B-Instruct-GPTQ-Int4 | 24GB | Excellent | Medium | Best Arabic quality |
| Qwen/Qwen3-32B-Instruct | 20GB | Excellent | Fast | Great balance |
| meta-llama/Llama-4-Maverick-17B-128E-Instruct | 16GB | Good | Fast | Coding tasks |
| deepseek-ai/DeepSeek-R2-Distill-Qwen-32B | 20GB | Good | Medium | Deep reasoning |
| microsoft/Phi-4-reasoning | 12GB | Medium | Very Fast | Quick tasks |

---

## Troubleshooting

| Issue | Solution |
|---|---|
| vLLM fails to start | Check GPU is enabled in notebook settings |
| Tunnel URL not working | Wait 2-3 minutes after tunnel opens |
| Session expired | Re-run the notebook from scratch |
| Out of GPU hours | Wait for weekly reset (Monday UTC) or use another account |
| Model too large | Use a smaller model (32B or 17B) |

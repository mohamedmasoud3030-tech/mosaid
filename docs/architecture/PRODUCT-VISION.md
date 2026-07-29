# Mosaid — Product Vision

**Date:** 2026-07-29  
**Status:** Accepted  
**Scope:** Architecture & product definition

---

## What is Mosaid?

Mosaid is a powerful, extensible **personal AI agent for digital business**. It helps a beginner user to:

1. **Understand** what a digital task involves (explain in simple Arabic/English)
2. **Learn** the required skills step-by-step
3. **Execute** the task using built-in tools and workflows
4. **Review** the output for quality
5. **Deliver** the final product (document, image, social post, code, etc.)
6. **Build a Portfolio** of completed work
7. **Find Opportunities** for income (jobs, freelance platforms)

---

## Core Principles

| Principle | Meaning |
|---|---|
| **Zero Cost** | Never spend money on AI inference. All providers are free-tier or self-hosted. |
| **Beginner-First** | Every task includes explanation, guidance, and teaching. |
| **Skill-Expandable** | New skills (work packs) can be added without modifying the core. |
| **Provider-Agnostic** | The cognitive engine never references a specific model or provider by name. |
| **Audit Everything** | Every decision, tool call, and approval is recorded in an append-only audit chain. |
| **Fail-Safe** | If anything is uncertain, stop and ask. Never guess on sensitive operations. |
| **Arabic-Native** | First-class Arabic (MSA + Egyptian dialect) support, not an afterthought. |

---

## Target Users

- **Primary:** Arabic-speaking beginner freelancers who want to earn income through digital work.
- **Secondary:** Developers who want an extensible AI agent platform to build on.
- **Tertiary:** Students learning digital skills (research, content, coding).

---

## Product Modes

1. **Telegram Bot** (primary interface): The user interacts via Telegram messages and commands.
2. **CLI** (developer mode): Direct command-line interaction for power users and testing.
3. **Android App** (future): On-device inference with offline capability via MLC LLM.

---

## What Mosaid Is NOT

- ❌ A general-purpose chatbot (it is goal-oriented)
- ❌ A paid SaaS (it is free and open-source)
- ❌ A model provider (it consumes free models, not hosts them)
- ❌ A social media manager that posts automatically without approval
- ❌ A code executor that runs arbitrary user-provided code

---

## Success Metrics

| Metric | Target |
|---|---|
| Zero-cost inference | $0.00 spent |
| Arabic quality | Usable for real freelance work |
| Work Pack coverage | 6 packs minimum in Phase 17 |
| Provider reliability | At least 3 providers available at all times |
| Test coverage | All packages have unit + race tests |
| Security | No secrets in git, no paid API keys, full audit trail |

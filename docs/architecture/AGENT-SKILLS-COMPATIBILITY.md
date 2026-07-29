# Mosaid — Agent Skills Compatibility

**Date:** 2026-07-29  
**Status:** Accepted  
**Phase:** 16

---

## Overview

The Skills Platform (Phase 8) already supports three skill types: `declarative`, `builtin`, and `mcp`. This document describes how the Agent Skills system (Cognitive Engine + Work Packs) integrates with the existing platform without breaking it.

---

## Existing Skills (Phase 8)

| Skill | Type | Status |
|---|---|---|
| `coding` | builtin | Active |
| `image-generation` | builtin | Active |
| `research` | builtin | Active |
| `social-publishing` | builtin | Active |

These skills continue to work unchanged. They are loaded by the existing `skills.Loader` and registered in the existing `skills.Registry`.

---

## New Agent Skills (Phase 16)

Agent Skills are higher-level compositions that use the Cognitive Engine. They are registered as `builtin` skills with a special `agent_compatible: true` flag in their manifest.

```json
{
  "id": "virtual-assistant",
  "version": "1.0.0",
  "type": "builtin",
  "description": "Beginner-friendly virtual assistant workflows",
  "required_tools": ["research", "memory"],
  "required_permissions": ["read", "write"],
  "timeout_seconds": 300,
  "agent_compatible": true,
  "workpack_ref": "virtual-assistant"
}
```

---

## Compatibility Rules

1. **Existing skills must not break.** No changes to `skills.Manifest`, `skills.Registry`, or `skills.Loader` that would invalidate existing manifests.

2. **New fields are optional.** Any new field added to `Manifest` (like `agent_compatible`) must be ignored by the existing loader if absent.

3. **Tool scoping is preserved.** Agent Skills cannot widen the tool scope beyond what their manifest declares.

4. **Approval binding is preserved.** Agent Skills respect the same approval policies as existing skills.

5. **Integrity hash is preserved.** Adding a new optional field changes the hash, so existing skills must be re-hashed when updated.

---

## Migration Path

1. Add `AgentCompatible bool` field to `Manifest` (optional, default false)
2. Add `WorkPackRef string` field to `Manifest` (optional)
3. Existing manifests without these fields continue to work (zero value = no change)
4. New Agent Skills include these fields
5. The Cognitive Engine checks `AgentCompatible` before using a skill through the agent loop

---

## Trust Pipeline for External Skills

External skills follow this pipeline:

```
discover → fetch_as_data → license_check → static_scan →
manifest_validation → required_tool_review →
risk_classification → hash_pinning → tests →
owner_approval → activation
```

**Absolutely forbidden for external skills:**
- ❌ `auto-download + auto-run`
- ❌ `eval`
- ❌ `curl | sh`
- ❌ `bash -c "..."`
- ❌ `sh -c "..."`

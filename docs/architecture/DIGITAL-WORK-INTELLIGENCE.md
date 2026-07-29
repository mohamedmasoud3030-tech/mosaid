# Mosaid — Digital Work Intelligence

**Date:** 2026-07-29  
**Status:** Accepted  
**Phase:** 17

---

## Purpose

Digital Work Intelligence is the layer that makes Mosaid useful for real-world digital business tasks. It translates user goals into structured workflows using **Work Packs** — pre-defined, tested, beginner-friendly task templates.

---

## Beginner Execution Flow

```
Explain → Prepare → Execute → Verify → Teach → Deliver
```

### Explain
- User describes what they want (in Arabic dialect, broken English, or vague terms)
- Mosaid explains: "This is a [task type]. Here's what it involves..."
- Lists: tools needed, estimated time, skill level required

### Prepare
- Gathers any missing inputs (file, URL, credentials, preferences)
- Creates a checklist of what will be done
- Asks for approval if needed

### Execute
- Runs the Work Pack workflow step by step
- Shows progress: "Step 2/5: Writing the content..."
- Records evidence at each step

### Verify
- Checks output against quality checklist
- Shows the result to user: "Here's what I created. Want me to change anything?"

### Teach
- Explains what was done and why
- Suggests how the user could do it themselves next time
- Links to relevant learning resources

### Deliver
- Provides final output in requested format
- Records to portfolio (if user has portfolio building enabled)
- Suggests next steps or related opportunities

---

## Work Pack Structure

```
workpacks/
├── virtual-assistant/
│   ├── WORKPACK.md          # Definition + workflow
│   ├── intake.json          # Input schema
│   ├── quality_checks.json  # Quality checklist
│   ├── templates/           # Output templates
│   └── tests/               # Validation tests
├── research/
├── content-writing/
├── coding-assistance/
├── social-media-management/
└── spreadsheets-reporting/
```

Each Work Pack contains:

| Component | Purpose |
|---|---|
| `WORKPACK.md` | Definition, workflow steps, dependencies |
| `intake.json` | JSON Schema for required inputs |
| `quality_checks.json` | Automated quality checks to run |
| `templates/` | Output templates (document, code, etc.) |
| `tests/` | Validation tests for the pack |
| `beginner_path.md` | Step-by-step guide for beginners |

---

## Completeness Criterion

A Work Pack is **COMPLETE** only when ALL of these exist:

```
intake + workflow + tools + quality_checks +
delivery + evidence + tests + beginner_path
```

---

## Work Packs by Priority

| # | Pack | Priority | Phase | Status |
|---|---|---|---|---|
| 1 | Virtual Assistant | Critical | 17 | Building |
| 2 | Research & Information | High | 17 | Building |
| 3 | Content Writing | High | 17 | Building |
| 4 | Coding Assistance | High | 17 | Building |
| 5 | Social Media Management | Medium | 17 | Building |
| 6 | Spreadsheets & Reporting | Medium | 17 | Building |
| 7 | Customer Support | Medium | Next | Planned |
| 8 | Lead Generation | Medium | Next | Planned |
| 9 | Ecommerce Operations | Low | Next | Planned |
| 10 | Basic Websites | Low | Next | Planned |
| 11 | Job & Freelance Apps | Low | Next | Planned |

#!/usr/bin/env python3
"""Fail-closed validation for the Mosaid Hermes pivot assets."""

from __future__ import annotations

import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
PIN = "b8ceba97ed0b2bf0255cc5c8c61c9110a026cda4"

REQUIRED = [
    "docs/pivot/HERMES-RUNTIME-DECISION.md",
    "docs/pivot/HERMES-UPSTREAM-PIN.md",
    "docs/pivot/MIGRATION-MAP.md",
    "docs/pivot/ORACLE-DEPLOYMENT-PLAN.md",
    "deploy/hermes/config.yaml.example",
    "deploy/hermes/mosaid.env.example",
    "product/hermes/SOUL.md",
    "product/hermes/.hermes.md",
    "product/skills/research/SKILL.md",
]

DANGEROUS_TOOLSETS = {
    "terminal",
    "file",
    "code_execution",
    "browser",
    "computer_use",
    "delegation",
    "cronjob",
    "image_gen",
    "video_gen",
    "x_search",
    "homeassistant",
    "spotify",
    "discord",
    "discord_admin",
}


def fail(message: str) -> None:
    print(f"Hermes pivot verification failed: {message}", file=sys.stderr)
    raise SystemExit(1)


def read(relative: str) -> str:
    path = ROOT / relative
    if not path.is_file():
        fail(f"missing required file: {relative}")
    return path.read_text(encoding="utf-8")


def list_under_key(text: str, parent: str, child: str) -> list[str]:
    lines = text.splitlines()
    parent_line = f"{parent}:"
    child_line = f"  {child}:"
    try:
        parent_index = lines.index(parent_line)
    except ValueError:
        fail(f"missing YAML key: {parent}")

    child_index = None
    for index in range(parent_index + 1, len(lines)):
        line = lines[index]
        if line and not line.startswith(" "):
            break
        if line == child_line:
            child_index = index
            break
    if child_index is None:
        fail(f"missing YAML key: {parent}.{child}")

    values: list[str] = []
    for line in lines[child_index + 1 :]:
        if line and not line.startswith("    "):
            break
        stripped = line.strip()
        if stripped.startswith("- "):
            values.append(stripped[2:].strip().strip('"\''))
    return values


def validate_skill(text: str) -> None:
    if not text.startswith("---\n"):
        fail("research Skill is missing YAML frontmatter")
    try:
        frontmatter = text.split("---\n", 2)[1]
    except IndexError:
        fail("research Skill frontmatter is not closed")

    required_fragments = [
        "name: mosaid-research",
        "version: 1.0.0",
        "platforms: [linux]",
        "requires_toolsets: [web]",
        "## When to Use",
        "## Procedure",
        "## Pitfalls",
        "## Verification",
    ]
    for fragment in required_fragments:
        if fragment not in text:
            fail(f"research Skill missing: {fragment}")

    match = re.search(r"^description:\s*(.+)$", frontmatter, flags=re.MULTILINE)
    if not match:
        fail("research Skill description is missing")
    description = match.group(1).strip().strip('"\'')
    if len(description) > 60:
        fail(f"research Skill description exceeds 60 chars: {len(description)}")


def validate_no_secrets(paths: list[str]) -> None:
    telegram_token = re.compile(r"(?<!REPLACE_WITH_)\b\d{6,}:[A-Za-z0-9_-]{20,}\b")
    api_key = re.compile(r"\b(?:sk|sk-or|gsk|AIza)[-_A-Za-z0-9]{16,}\b")
    private_key = "-----BEGIN PRIVATE KEY-----"
    for relative in paths:
        text = read(relative)
        if telegram_token.search(text):
            fail(f"possible Telegram token in {relative}")
        if api_key.search(text):
            fail(f"possible API key in {relative}")
        if private_key in text:
            fail(f"private key material in {relative}")


def main() -> None:
    for relative in REQUIRED:
        read(relative)

    env = read("deploy/hermes/mosaid.env.example")
    pin_doc = read("docs/pivot/HERMES-UPSTREAM-PIN.md")
    config = read("deploy/hermes/config.yaml.example")
    soul = read("product/hermes/SOUL.md")
    policy = read("product/hermes/.hermes.md")
    skill = read("product/skills/research/SKILL.md")

    env_pin = re.search(r"^HERMES_REF=([0-9a-f]{40})$", env, flags=re.MULTILINE)
    if not env_pin or env_pin.group(1) != PIN:
        fail("environment template does not pin the reviewed Hermes commit")
    if PIN not in pin_doc:
        fail("upstream pin document disagrees with environment template")
    if "HERMES_REF=main" in env or "HERMES_REF=master" in env:
        fail("floating Hermes branch is forbidden")
    if re.search(r"^[A-Z0-9_]+_FILE=", env, flags=re.MULTILINE):
        fail("unsupported *_FILE variables found in Hermes environment template")

    required_config = [
        'provider: "custom"',
        'default: "${MODEL_NAME}"',
        'base_url: "${MODEL_BASE_URL}"',
        "external_dirs:",
        "- /opt/mosaid/product/skills",
        "guard_agent_created: true",
        "docker_network: false",
    ]
    for fragment in required_config:
        if fragment not in config:
            fail(f"restricted Hermes config missing: {fragment}")
    if config.count("write_approval: true") < 2:
        fail("both Skill and memory writes must require approval")

    telegram_tools = set(list_under_key(config, "platform_toolsets", "telegram"))
    leaked = telegram_tools & DANGEROUS_TOOLSETS
    if leaked:
        fail(f"dangerous Telegram toolsets enabled: {sorted(leaked)}")
    required_read_tools = {"web", "skills", "memory", "session_search", "clarify"}
    missing = required_read_tools - telegram_tools
    if missing:
        fail(f"required read-only Telegram toolsets missing: {sorted(missing)}")

    disabled = set(list_under_key(config, "agent", "disabled_toolsets"))
    missing_denials = DANGEROUS_TOOLSETS - disabled
    if missing_denials:
        fail(f"dangerous toolsets are not globally disabled: {sorted(missing_denials)}")

    if "مساعد" not in soul or "A model cannot approve" in soul:
        fail("SOUL.md identity is missing or unexpectedly replaced")
    for phrase in ["Tools are deny-by-default", "Paid fallback is forbidden", "A model cannot approve"]:
        if phrase not in policy:
            fail(f"project policy missing invariant: {phrase}")

    validate_skill(skill)
    validate_no_secrets(REQUIRED + ["README.md"])

    print("Hermes pivot assets verified")


if __name__ == "__main__":
    main()

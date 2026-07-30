#!/usr/bin/env python3
"""Fail-closed validation for the Mosaid Hermes pivot assets."""

from __future__ import annotations

import re
import shutil
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
PIN = "b8ceba97ed0b2bf0255cc5c8c61c9110a026cda4"

AGENTENV_ADR = "docs/pivot/AGENTENV-EXECUTION-BACKEND-DECISION.md"

REQUIRED = [
    "docs/pivot/HERMES-RUNTIME-DECISION.md",
    "docs/pivot/HERMES-UPSTREAM-PIN.md",
    "docs/pivot/MIGRATION-MAP.md",
    "docs/pivot/ORACLE-DEPLOYMENT-PLAN.md",
    AGENTENV_ADR,
    "deploy/hermes/config.yaml.example",
    "deploy/hermes/mosaid.env.example",
    "deploy/hermes/stage-release.sh",
    "deploy/hermes/preflight.sh",
    "deploy/hermes/mosaid-hermes.service",
    "product/hermes/SOUL.md",
    "product/hermes/.hermes.md",
    "product/skills/research/SKILL.md",
    "deploy/oracle/PROVISIONING-RUNBOOK.md",
    "deploy/oracle/bootstrap-host.sh",
    "deploy/oracle/collect-instance-facts.sh",
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


# Files that describe what actually gets installed and run on the Oracle host.
# Prose documentation is deliberately excluded: an ADR must be able to name a
# forbidden pattern in order to forbid it.
PRODUCTION_PATHS = [
    "deploy/hermes/config.yaml.example",
    "deploy/hermes/mosaid.env.example",
    "deploy/hermes/stage-release.sh",
    "deploy/hermes/preflight.sh",
    "deploy/hermes/mosaid-hermes.service",
    "deploy/oracle/bootstrap-host.sh",
    "deploy/oracle/collect-instance-facts.sh",
    ".github/workflows/product-ci.yml",
    ".github/workflows/phase0-ci.yml",
]

# Files that declare what the runtime *is*, as opposed to scripts that decide
# what to install. Any AgentENV mention in these is wiring, not a refusal.
DECLARATIVE_RUNTIME_PATHS = [
    "deploy/hermes/config.yaml.example",
    "deploy/hermes/mosaid.env.example",
    "deploy/hermes/mosaid-hermes.service",
    ".github/workflows/product-ci.yml",
    ".github/workflows/phase0-ci.yml",
]

# Shell scripts that must parse before they are ever run on the instance.
SHELL_SCRIPTS = [
    "deploy/hermes/stage-release.sh",
    "deploy/hermes/preflight.sh",
    "deploy/oracle/bootstrap-host.sh",
    "deploy/oracle/collect-instance-facts.sh",
]

# Documents that must point at the ADR instead of restating or contradicting it.
ADR_REFERENCES = [
    "README.md",
    "docs/pivot/HERMES-RUNTIME-DECISION.md",
    "docs/pivot/MIGRATION-MAP.md",
    "docs/pivot/ORACLE-DEPLOYMENT-PLAN.md",
    "deploy/hermes/README.md",
]

ADR_REQUIRED_SECTIONS = [
    "## 1. What AgentENV is",
    "## 2. Why AgentENV does not replace Oracle",
    "## 3. Why AgentENV does not replace Hermes",
    "## 4. Where AgentENV could fit in the future",
    "## 5. Why AgentENV is not added now",
    "## 6. Accepted execution order",
    "## 7. Security requirements for any future AgentENV use",
    "## 8. Conditions for future adoption",
    "## 9. Rejection and rollback conditions",
    "## 10. Architecture diagrams",
]

ADR_REQUIRED_GATE_FACTS = [
    "Ubuntu 24.04",
    "6.8+",
    "/dev/kvm",
    "/dev/ublk-control",
    "Nested virtualization",
    "CAP_NET_ADMIN",
    "CAP_SYS_ADMIN",
]

# Tokens that would indicate AgentENV entering the live deployment path.
AGENTENV_PRODUCTION_MARKERS = [
    "agentenv",
    "aenv_api_url",
    "aenv-server",
    "kvcache-ai",
    "e2b_api_url",
    "firecracker",
    "overlaybd",
]


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


def validate_agentenv_adr() -> None:
    """The AgentENV decision must exist and stay substantive, not a stub."""
    text = read(AGENTENV_ADR)

    for section in ADR_REQUIRED_SECTIONS:
        if section not in text:
            fail(f"AgentENV ADR missing section: {section}")

    for fact in ADR_REQUIRED_GATE_FACTS:
        if fact not in text:
            fail(f"AgentENV ADR missing adoption-gate requirement: {fact}")

    # The decision itself, not merely a mention.
    if "deferred, not adopted" not in text:
        fail("AgentENV ADR does not record a deferred/not-adopted status")

    # The layering conclusions that other documents depend on.
    for claim in [
        "https://github.com/kvcache-ai/AgentENV",
        "does not support authorization",
        "127.0.0.1",
    ]:
        if claim not in text:
            fail(f"AgentENV ADR missing required content: {claim}")

    if len(text.splitlines()) < 120:
        fail("AgentENV ADR is too short to carry the required decision content")


def validate_adr_is_linked() -> None:
    """Other documents must reference the ADR rather than duplicate it."""
    target = "AGENTENV-EXECUTION-BACKEND-DECISION.md"
    for relative in ADR_REFERENCES:
        if target not in read(relative):
            fail(f"{relative} does not link to the AgentENV decision record")


def validate_no_agentenv_in_production() -> None:
    """AgentENV must not appear in declarative runtime configuration.

    Applied to config, environment, unit and workflow files, where the runtime
    is *declared*: any mention there is wiring. The provisioning shell scripts
    are excluded because they must name AgentENV in order to detect and refuse
    it; they are instead held to the stricter install-command checks in
    validate_oracle_provisioning and validate_stage_release_isolation.
    """
    for relative in DECLARATIVE_RUNTIME_PATHS:
        path = ROOT / relative
        if not path.is_file():
            continue
        lowered = path.read_text(encoding="utf-8").lower()
        for marker in AGENTENV_PRODUCTION_MARKERS:
            if marker in lowered:
                fail(f"AgentENV marker '{marker}' found in production path: {relative}")


def validate_stage_release_isolation() -> None:
    """The staging script must not install or enable any sandbox runtime."""
    stage = read("deploy/hermes/stage-release.sh")
    lowered = stage.lower()

    forbidden_commands = [
        "install.sh",
        "install-cli.sh",
        "docker-setup.sh",
        "aenv ",
        "docker run",
        "docker pull",
        "apt-get install docker",
        "apt install docker",
        "systemctl start aenv",
        "modprobe kvm",
    ]
    for fragment in forbidden_commands:
        if fragment in lowered:
            fail(f"stage script must not install or run a sandbox runtime: {fragment}")

    # Unpinned remote code execution as root is forbidden outright.
    if re.search(r"curl[^\n|]*\|\s*(sudo\s+)?(ba)?sh", lowered):
        fail("stage script must not pipe a remote script into a shell")
    if re.search(r"wget[^\n|]*\|\s*(sudo\s+)?(ba)?sh", lowered):
        fail("stage script must not pipe a downloaded script into a shell")


def validate_no_sandbox_exposure() -> None:
    """No world-facing execution port, KVM device or privileged container.

    Device and privilege wiring is checked only in declarative runtime files;
    the provisioning scripts legitimately *probe* for /dev/kvm to report and
    refuse it, and are covered by validate_no_sandbox_install instead.
    """
    for relative in PRODUCTION_PATHS:
        path = ROOT / relative
        if not path.is_file():
            continue
        text = path.read_text(encoding="utf-8")
        lowered = text.lower()

        if relative in DECLARATIVE_RUNTIME_PATHS:
            if "--privileged" in lowered:
                fail(f"privileged container flag found in production path: {relative}")
            if "/dev/kvm" in lowered or "/dev/ublk-control" in lowered:
                fail(f"virtualization device wired into production path: {relative}")
            if re.search(r"-v\s+/dev:/dev", lowered):
                fail(f"host /dev mount found in production path: {relative}")

        # Port 8000 must never be published to a non-loopback address.
        for match in re.finditer(r"(\d{1,3}(?:\.\d{1,3}){3}|\*|::)?:?8000\b", text):
            window = text[max(0, match.start() - 60) : match.end() + 20].lower()
            if any(
                token in window
                for token in ("0.0.0.0:8000", "*:8000", ":::8000", "-p 8000", "--publish 8000")
            ):
                fail(f"port 8000 is exposed in production path: {relative}")


def validate_service_isolation() -> None:
    """The systemd unit must not gain virtualization or privileged access."""
    unit = read("deploy/hermes/mosaid-hermes.service")

    if re.search(r"^DeviceAllow=.*kvm", unit, flags=re.MULTILINE | re.IGNORECASE):
        fail("systemd unit grants KVM device access")
    if re.search(r"^PrivateDevices=false", unit, flags=re.MULTILINE):
        fail("systemd unit disables PrivateDevices")
    if re.search(r"^(Ambient|Capability(Bounding)?)Capabilities?=\s*CAP_", unit, flags=re.MULTILINE):
        fail("systemd unit grants Linux capabilities")

    # Capability sets must remain empty, not merely present.
    for key in ("CapabilityBoundingSet", "AmbientCapabilities"):
        match = re.search(rf"^{key}=(.*)$", unit, flags=re.MULTILINE)
        if match is None:
            fail(f"systemd unit missing {key}")
        if match.group(1).strip():
            fail(f"systemd unit must keep {key} empty; found: {match.group(1).strip()}")

    if "ExecStart=/opt/mosaid/current/.venv/bin/hermes gateway" not in unit:
        fail("systemd unit must start only the Hermes gateway")
    if re.search(r"^ExecStart(Pre|Post)?=.*(docker|aenv|podman)", unit, flags=re.MULTILINE):
        fail("systemd unit must not invoke a container or sandbox runtime")


def validate_no_autostart(paths: list[str]) -> None:
    """Staging must never start, enable or auto-run the service."""
    for relative in paths:
        path = ROOT / relative
        if not path.is_file():
            continue
        text = path.read_text(encoding="utf-8")
        if re.search(r"\bsystemctl\s+(--now\s+)?(start|restart|enable)\b", text):
            fail(f"service is started or enabled automatically in {relative}")
        if re.search(r"\bsystemctl\s+enable\s+--now\b", text):
            fail(f"service is enabled at boot automatically in {relative}")


def validate_oracle_provisioning() -> None:
    """Host provisioning must stay pinned, verified and side-effect free."""
    bootstrap = read("deploy/oracle/bootstrap-host.sh")
    collector = read("deploy/oracle/collect-instance-facts.sh")
    runbook = read("deploy/oracle/PROVISIONING-RUNBOOK.md")

    # uv must be pinned to an exact version and checksum-verified, never piped.
    if not re.search(r'^readonly UV_VERSION="\d+\.\d+\.\d+"$', bootstrap, flags=re.MULTILINE):
        fail("bootstrap does not pin an exact uv version")
    checksums = re.findall(r'^readonly UV_SHA256_[A-Z0-9_]+="([0-9a-f]{64})"$', bootstrap, flags=re.MULTILINE)
    if len(checksums) < 2:
        fail("bootstrap must pin a SHA-256 for each supported architecture")
    if "sha256sum" not in bootstrap or "checksum mismatch" not in bootstrap:
        fail("bootstrap does not verify the downloaded uv checksum")

    # The staging script owns Hermes installation; the bootstrap must not.
    for fragment in ("hermes-agent.git", "uv sync"):
        if fragment in bootstrap:
            fail(f"bootstrap must not install Hermes itself: {fragment}")

    # Provisioning must refuse a contaminated host rather than proceed.
    for guard in ("Docker is running", "AgentENV detected", "port 8000 is already listening"):
        if guard not in bootstrap:
            fail(f"bootstrap is missing a first-gate guard: {guard}")

    # The collector is read-only and must not leak identifying data.
    for forbidden in ("curl ", "wget ", "apt-get", "dnf ", "useradd", "install -d", "systemctl start"):
        if forbidden in collector:
            fail(f"instance-facts collector must stay read-only: {forbidden}")

    # Identifying values must never be executed into the report. Comments and
    # the report's own disclaimer may mention them; command substitutions and
    # address-revealing invocations may not.
    for pattern, label in (
        (r"\$\(\s*hostname", "hostname"),
        (r"\bhostname\s+-[iIf]", "hostname address lookup"),
        (r"\bip\s+addr\b", "ip addr"),
        (r"\bifconfig\b", "ifconfig"),
        (r"\bcurl\b[^\n]*169\.254\.169\.254", "instance metadata service"),
        (r"\boci\s+compute\b", "OCI CLI instance query"),
    ):
        if re.search(pattern, collector):
            fail(f"instance-facts collector must not report {label}")

    # It must publish port numbers only, never the bound addresses.
    if re.search(r"^\s*ss\b(?![^\n]*\bawk\b)", collector, flags=re.MULTILINE):
        fail("instance-facts collector must strip addresses from socket output")

    if "Auth Token" not in runbook or "0600" not in runbook:
        fail("provisioning runbook must state the Auth Token and secret-permission rules")

    # The provisioning scripts are exempt from the declarative marker scan
    # because they name AgentENV to refuse it, so hold them to the stronger
    # rule instead: they may never actually install or launch one.
    for relative in ("deploy/oracle/bootstrap-host.sh", "deploy/oracle/collect-instance-facts.sh"):
        validate_no_sandbox_install(relative)


def strip_shell_comments(text: str) -> str:
    """Drop whole-line shell comments so checks test code, not prose."""
    return "\n".join(
        line for line in text.splitlines() if not line.lstrip().startswith("#")
    )


def validate_no_sandbox_install(relative: str) -> None:
    """A script may detect a sandbox runtime, but never install or start one."""
    lowered = strip_shell_comments(read(relative)).lower()

    forbidden = [
        r"\baenv\s+(auth|pull|start|exec)\b",
        r"\baenv-server\b",
        r"\bsystemctl\s+(start|enable)\s+aenv\b",
        r"\bdocker\s+(run|pull|start)\b",
        r"\bdocker-setup\.sh\b",
        r"\binstall-cli\.sh\b",
        r"\b(apt-get|apt|dnf|yum)\s+install\b[^\n]*\bdocker\b",
        r"\bmodprobe\s+kvm\b",
        r"--privileged\b",
    ]
    for pattern in forbidden:
        match = re.search(pattern, lowered)
        if match:
            fail(
                f"{relative} must not install or run a sandbox runtime: "
                f"{match.group(0).strip()}"
            )

    if re.search(r"(curl|wget)[^\n|]*\|\s*(sudo\s+)?(ba)?sh", lowered):
        fail(f"{relative} must not pipe a remote script into a shell")

    # Any download must be checksum-verified before it is trusted.
    if re.search(r"^\s*curl\b", lowered, flags=re.MULTILINE) and "sha256sum" not in lowered:
        fail(f"{relative} downloads a file without verifying a checksum")


def validate_shell_syntax() -> None:
    """Deployment shell scripts must parse."""
    bash = shutil.which("bash")
    if bash is None:
        return
    for relative in SHELL_SCRIPTS:
        path = ROOT / relative
        if not path.is_file():
            fail(f"missing shell script: {relative}")
        result = subprocess.run(
            [bash, "-n", str(path)],
            capture_output=True,
            text=True,
            check=False,
        )
        if result.returncode != 0:
            fail(f"shell syntax error in {relative}: {result.stderr.strip()}")


def validate_no_secrets(paths: list[str]) -> None:
    telegram_token = re.compile(r"(?<!REPLACE_WITH_)\b\d{6,}:[A-Za-z0-9_-]{20,}\b")
    api_key = re.compile(r"\b(?:sk|sk-or|gsk|AIza)[-_A-Za-z0-9]{16,}\b")
    github_pat = re.compile(r"\b(?:ghp|gho|ghu|ghs|ghr|github_pat)_[A-Za-z0-9_]{20,}\b")
    private_key_headers = (
        "-----BEGIN PRIVATE KEY-----",
        "-----BEGIN RSA PRIVATE KEY-----",
        "-----BEGIN OPENSSH PRIVATE KEY-----",
        "-----BEGIN EC PRIVATE KEY-----",
    )
    for relative in paths:
        text = read(relative)
        if telegram_token.search(text):
            fail(f"possible Telegram token in {relative}")
        if api_key.search(text):
            fail(f"possible API key in {relative}")
        if github_pat.search(text):
            fail(f"possible GitHub token in {relative}")
        for header in private_key_headers:
            if header in text:
                fail(f"private key material in {relative}")


def main() -> None:
    for relative in REQUIRED:
        read(relative)

    env = read("deploy/hermes/mosaid.env.example")
    pin_doc = read("docs/pivot/HERMES-UPSTREAM-PIN.md")
    config = read("deploy/hermes/config.yaml.example")
    stage = read("deploy/hermes/stage-release.sh")
    preflight = read("deploy/hermes/preflight.sh")
    unit = read("deploy/hermes/mosaid-hermes.service")
    soul = read("product/hermes/SOUL.md")
    policy = read("product/hermes/.hermes.md")
    skill = read("product/skills/research/SKILL.md")

    env_pin = re.search(r"^HERMES_REF=([0-9a-f]{40})$", env, flags=re.MULTILINE)
    if not env_pin or env_pin.group(1) != PIN:
        fail("environment template does not pin the reviewed Hermes commit")
    for label, text in {
        "upstream pin document": pin_doc,
        "stage script": stage,
        "preflight": preflight,
    }.items():
        if PIN not in text:
            fail(f"{label} disagrees with the reviewed Hermes commit")
    if "HERMES_REF=main" in env or "HERMES_REF=master" in env:
        fail("floating Hermes branch is forbidden")
    if re.search(r"^[A-Z0-9_]+_FILE=", env, flags=re.MULTILINE):
        fail("unsupported *_FILE variables found in Hermes environment template")

    if re.search(r"\bsystemctl\s+(?:start|restart|enable|enable\s+--now)\b", stage):
        fail("stage script must not start or enable the service")
    for fragment in [
        'uv sync --project "${release_dir}" --frozen --no-dev --extra messaging',
        'mv "${temporary_dir}" "${release_dir}"',
        'install -o root -g root -m 0555',
        'system user \'${MOSAID_USER}\' does not exist',
    ]:
        if fragment not in stage:
            fail(f"stage script missing invariant: {fragment}")

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

    for fragment in [
        "User=mosaid",
        "Group=mosaid",
        "ExecStartPre=/opt/mosaid/bin/preflight-hermes",
        "ExecStart=/opt/mosaid/current/.venv/bin/hermes gateway",
        "NoNewPrivileges=true",
        "ProtectSystem=strict",
        "ReadOnlyPaths=/opt/mosaid",
        "ReadWritePaths=/var/lib/mosaid",
    ]:
        if fragment not in unit:
            fail(f"systemd unit missing hardening: {fragment}")

    if "MOSAID_MAX_SPEND_USD" not in preflight or "TELEGRAM_ALLOWED_USERS" not in preflight:
        fail("preflight does not enforce billing and owner identity")
    if "REPLACE_" not in env:
        fail("environment example must contain explicit safe placeholders")

    if "مساعد" not in soul or "A model cannot approve" in soul:
        fail("SOUL.md identity is missing or unexpectedly replaced")
    for phrase in ["Tools are deny-by-default", "Paid fallback is forbidden", "A model cannot approve"]:
        if phrase not in policy:
            fail(f"project policy missing invariant: {phrase}")

    validate_skill(skill)

    # AgentENV execution-backend decision and its enforcement.
    validate_agentenv_adr()
    validate_adr_is_linked()
    validate_no_agentenv_in_production()
    validate_stage_release_isolation()
    validate_no_sandbox_exposure()
    validate_service_isolation()
    validate_no_autostart(
        ["deploy/hermes/stage-release.sh", "deploy/oracle/bootstrap-host.sh"]
    )
    validate_oracle_provisioning()
    validate_shell_syntax()

    validate_no_secrets(REQUIRED + ADR_REFERENCES)

    print("Hermes pivot assets verified")


if __name__ == "__main__":
    main()

#!/usr/bin/env python3
"""Generate a CycloneDX SBOM and an evidence-oriented license report for a Go binary.

The component set comes from `go version -m <binary>`, i.e. modules actually
linked into the binary, rather than every module present in go.mod. License
classification is best effort and must not be treated as legal advice.
"""
from __future__ import annotations

import argparse
import datetime as dt
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import urllib.parse


def run(*args: str, cwd: Path | None = None) -> str:
    return subprocess.check_output(args, cwd=cwd, text=True, stderr=subprocess.STDOUT)


def sha256(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as f:
        for block in iter(lambda: f.read(1024 * 1024), b""):
            h.update(block)
    return h.hexdigest()


def parse_build_info(binary: Path) -> tuple[str, list[dict[str, str]], dict[str, str], str]:
    raw = run("go", "version", "-m", str(binary))
    modules: list[dict[str, str]] = []
    build: dict[str, str] = {}
    go_version = "unknown"
    for line in raw.splitlines():
        fields = line.lstrip("\t").split("\t")
        if not fields:
            continue
        if len(fields) == 1 and fields[0].startswith(str(binary) + ": "):
            go_version = fields[0].rsplit(": ", 1)[-1]
        elif fields[0] == "dep" and len(fields) >= 3:
            modules.append({"path": fields[1], "version": fields[2], "sum": fields[3] if len(fields) > 3 else ""})
        elif fields[0] == "build" and len(fields) >= 3:
            build[fields[1]] = fields[2]
    return go_version, modules, build, raw


def module_inventory(source: Path) -> dict[tuple[str, str], dict]:
    raw = run("go", "list", "-m", "-json", "all", cwd=source)
    decoder = json.JSONDecoder()
    result: dict[tuple[str, str], dict] = {}
    i = 0
    while i < len(raw):
        while i < len(raw) and raw[i].isspace():
            i += 1
        if i >= len(raw):
            break
        obj, i = decoder.raw_decode(raw, i)
        result[(obj.get("Path", ""), obj.get("Version", ""))] = obj
    return result


def classify_license(text: str) -> str:
    s = text.lower()
    if "eclipse public license" in s and ("v 2.0" in s or "version 2.0" in s):
        return "EPL-2.0 OR EDL-1.0"
    if "apache license" in s and "version 2.0" in s:
        return "Apache-2.0"
    if "permission is hereby granted, free of charge" in s and "the software is provided \"as is\"" in s:
        return "MIT"
    if "redistribution and use in source and binary forms" in s:
        if "neither the name" in s:
            return "BSD-3-Clause"
        return "BSD-2-Clause"
    if "mozilla public license version 2.0" in s:
        return "MPL-2.0"
    if "gnu lesser general public license" in s:
        return "LGPL"
    if "gnu general public license" in s:
        return "GPL"
    if "isc license" in s or "permission to use, copy, modify, and/or distribute" in s:
        return "ISC"
    return "UNKNOWN"


def find_license(module_dir: str) -> tuple[str, str, str]:
    if not module_dir:
        return "", "", "UNKNOWN"
    root = Path(module_dir)
    names = ("LICENSE", "LICENSE.txt", "LICENSE.md", "COPYING", "COPYING.txt", "NOTICE")
    for name in names:
        p = root / name
        if p.is_file():
            data = p.read_bytes()
            text = data.decode("utf-8", "replace")
            return name, hashlib.sha256(data).hexdigest(), classify_license(text)
    return "", "", "NOT_FOUND"


def purl(path: str, version: str) -> str:
    return f"pkg:golang/{urllib.parse.quote(path, safe='/')}@{urllib.parse.quote(version, safe='.+-')}"


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--binary", type=Path, required=True)
    ap.add_argument("--source", type=Path, required=True)
    ap.add_argument("--output-dir", type=Path, required=True)
    ap.add_argument("--timestamp", default="2026-06-30T09:42:07Z", help="Stable evidence timestamp")
    ns = ap.parse_args()
    ns.output_dir.mkdir(parents=True, exist_ok=True)

    go_version, linked, build, raw = parse_build_info(ns.binary)
    inventory = module_inventory(ns.source)
    binary_hash = sha256(ns.binary)

    components = []
    license_rows = []
    module_rows = []
    for mod in sorted(linked, key=lambda x: x["path"]):
        info = inventory.get((mod["path"], mod["version"]), {})
        lic_path, lic_hash, lic = find_license(info.get("Dir", ""))
        comp = {
            "type": "library",
            "name": mod["path"],
            "version": mod["version"],
            "bom-ref": purl(mod["path"], mod["version"]),
            "purl": purl(mod["path"], mod["version"]),
            "properties": [
                {"name": "go.module.sum", "value": mod["sum"]},
                {"name": "phase0.license_detection", "value": lic},
            ],
        }
        if lic not in {"UNKNOWN", "NOT_FOUND"}:
            comp["licenses"] = [{"license": {"id": lic}}]
        components.append(comp)
        module_rows.append((mod["path"], mod["version"], mod["sum"], comp["purl"]))
        license_rows.append((mod["path"], mod["version"], lic, lic_hash, lic_path))

    serial_seed = hashlib.sha256((binary_hash + "\n" + "\n".join(c["bom-ref"] for c in components)).encode()).hexdigest()
    serial = f"urn:uuid:{serial_seed[:8]}-{serial_seed[8:12]}-{serial_seed[12:16]}-{serial_seed[16:20]}-{serial_seed[20:32]}"
    sbom = {
        "bomFormat": "CycloneDX",
        "specVersion": "1.5",
        "serialNumber": serial,
        "version": 1,
        "metadata": {
            "timestamp": ns.timestamp,
            "tools": {"components": [{"type": "application", "name": "phase0-generate-sbom.py", "version": "1"}]},
            "component": {
                "type": "application",
                "name": ns.binary.name,
                "version": "v0.3.1-phase0.1",
                "hashes": [{"alg": "SHA-256", "content": binary_hash}],
                "properties": [
                    {"name": "go.version", "value": go_version},
                    {"name": "upstream.commit", "value": "2cf030d2fd3b871d7ec17e3be34c24688aac76da"},
                    {"name": "build.GOOS", "value": build.get("GOOS", "unknown")},
                    {"name": "build.GOARCH", "value": build.get("GOARCH", "unknown")},
                ],
            },
        },
        "components": components,
    }
    (ns.output_dir / "sbom.cdx.json").write_text(json.dumps(sbom, indent=2, sort_keys=True) + "\n")
    (ns.output_dir / "go-version-m.txt").write_text(raw)
    with (ns.output_dir / "linked-modules.tsv").open("w") as f:
        f.write("module\tversion\tgo_sum\tpurl\n")
        for row in module_rows:
            f.write("\t".join(row) + "\n")
    with (ns.output_dir / "license-report.tsv").open("w") as f:
        f.write("module\tversion\tdetected_spdx\tlicense_sha256\tlicense_evidence_filename\n")
        for row in license_rows:
            f.write("\t".join(row) + "\n")
    summary = {
        "binary": ns.binary.name,
        "binary_sha256": binary_hash,
        "go_version": go_version,
        "linked_module_count": len(linked),
        "license_counts": {},
        "note": "License detection is best effort and not legal advice.",
    }
    for _, _, lic, _, _ in license_rows:
        summary["license_counts"][lic] = summary["license_counts"].get(lic, 0) + 1
    (ns.output_dir / "sbom-summary.json").write_text(json.dumps(summary, indent=2, sort_keys=True) + "\n")


if __name__ == "__main__":
    main()

#!/usr/bin/env python3
import argparse
import json
import subprocess
from pathlib import Path


def json_stream(text):
    decoder = json.JSONDecoder()
    index = 0
    while index < len(text):
        while index < len(text) and text[index].isspace():
            index += 1
        if index >= len(text):
            break
        value, index = decoder.raw_decode(text, index)
        yield value


def classify(text):
    lower = text.lower()
    if "mozilla public license" in lower:
        return "MPL-2.0"
    if "apache license" in lower and "version 2.0" in lower:
        return "Apache-2.0"
    if "permission is hereby granted, free of charge" in lower:
        return "MIT"
    if "redistribution and use in source and binary forms" in lower:
        return "BSD"
    if "permission to use, copy, modify, and/or distribute this software" in lower:
        return "ISC"
    if "gnu lesser general public license" in lower:
        return "LGPL"
    if "gnu general public license" in lower:
        return "GPL"
    return "UNKNOWN"


def find_license(directory):
    root = Path(directory)
    candidates = []
    for pattern in ("LICENSE*", "COPYING*", "NOTICE*"):
        candidates.extend(root.glob(pattern))
    for path in sorted(set(candidates)):
        if path.is_file() and not path.is_symlink() and path.stat().st_size <= 1024 * 1024:
            text = path.read_text(encoding="utf-8", errors="replace")
            license_id = classify(text)
            if license_id != "UNKNOWN":
                return license_id, path.name
    return "UNKNOWN", "-"


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", required=True)
    args = parser.parse_args()
    raw = subprocess.check_output(["go", "list", "-m", "-json", "all"], text=True)
    rows = []
    failures = []
    for module in sorted(json_stream(raw), key=lambda item: item["Path"]):
        directory = module.get("Dir")
        version = module.get("Version") or "workspace"
        if not directory and version != "workspace":
            downloaded = subprocess.check_output(["go", "mod", "download", "-json", f"{module['Path']}@{version}"], text=True)
            directory = json.loads(downloaded).get("Dir")
        if not directory:
            failures.append(module["Path"] + ": module directory unavailable")
            continue
        license_id, source = find_license(directory)
        rows.append((module["Path"], version, license_id, source))
        if license_id in {"UNKNOWN", "GPL", "LGPL"}:
            failures.append(f"{module['Path']}: unapproved license {license_id}")
    output = Path(args.output)
    output.parent.mkdir(parents=True, exist_ok=True)
    lines = ["module\tversion\tlicense\tsource"] + ["\t".join(row) for row in rows]
    output.write_text("\n".join(lines) + "\n", encoding="utf-8")
    if failures:
        raise SystemExit("license verification failed: " + "; ".join(failures))
    print(f"License verification: PASS ({len(rows)} modules)")


if __name__ == "__main__":
    main()

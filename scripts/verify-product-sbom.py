#!/usr/bin/env python3
import argparse
import json
import subprocess
from pathlib import Path


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--input", required=True)
    args = parser.parse_args()
    data = json.loads(Path(args.input).read_text(encoding="utf-8"))
    if data.get("bomFormat") != "CycloneDX" or data.get("specVersion") != "1.5" or data.get("version") != 1:
        raise SystemExit("invalid CycloneDX header")
    components = data.get("components")
    if not isinstance(components, list) or not components:
        raise SystemExit("SBOM components missing")
    names = set()
    refs = set()
    for item in components:
        name = item.get("name")
        ref = item.get("bom-ref")
        if not name or not ref or name in names or ref in refs:
            raise SystemExit("duplicate or malformed SBOM component")
        encoded = json.dumps(item)
        if any(marker in encoded for marker in ('"Dir"', 'GOMODCACHE', '/home/', '\\Users\\')):
            raise SystemExit("SBOM contains local path")
        names.add(name)
        refs.add(ref)
    listed = set(subprocess.check_output(["go", "list", "-m", "-f", "{{.Path}}", "all"], text=True).splitlines())
    if names != listed:
        missing = sorted(listed - names)
        extra = sorted(names - listed)
        raise SystemExit(f"SBOM module mismatch missing={missing} extra={extra}")
    print(f"SBOM verification: PASS ({len(components)} modules)")


if __name__ == "__main__":
    main()

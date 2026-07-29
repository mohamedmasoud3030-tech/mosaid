#!/usr/bin/env python3
import argparse
import base64
import hashlib
import json
import subprocess
import urllib.parse
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


def module_ref(module):
    version = module.get("Version") or "workspace"
    return "pkg:golang/" + urllib.parse.quote(module["Path"], safe="/") + "@" + urllib.parse.quote(version, safe="")


def component(module, sums):
    version = module.get("Version") or "workspace"
    item = {
        "type": "library" if module.get("Main") is not True else "application",
        "bom-ref": module_ref(module),
        "name": module["Path"],
        "version": version,
        "purl": module_ref(module),
        "properties": [],
    }
    checksum = sums.get((module["Path"], version), "")
    if checksum.startswith("h1:"):
        try:
            digest = base64.b64decode(checksum[3:]).hex()
            item["hashes"] = [{"alg": "SHA-256", "content": digest}]
        except Exception:
            pass
        item["properties"].append({"name": "golang:module:sum", "value": checksum})
    if not item["properties"]:
        item.pop("properties")
    return item


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", required=True)
    args = parser.parse_args()
    raw = subprocess.check_output(["go", "list", "-m", "-json", "all"], text=True)
    modules = sorted(json_stream(raw), key=lambda module: module["Path"])
    sums = {}
    go_sum = Path("go.sum")
    if go_sum.exists():
        for line in go_sum.read_text(encoding="utf-8").splitlines():
            fields = line.split()
            if len(fields) == 3 and not fields[1].endswith("/go.mod"):
                sums[(fields[0], fields[1])] = fields[2]
    main_module = next(module for module in modules if module.get("Main"))
    components = [component(module, sums) for module in modules]
    refs = sorted(item["bom-ref"] for item in components if item["bom-ref"] != module_ref(main_module))
    graph_identity = "\n".join(item["bom-ref"] for item in components).encode()
    serial = hashlib.sha256(graph_identity).hexdigest()
    document = {
        "bomFormat": "CycloneDX",
        "specVersion": "1.5",
        "serialNumber": "urn:uuid:" + serial[:8] + "-" + serial[8:12] + "-5" + serial[13:16] + "-a" + serial[17:20] + "-" + serial[20:32],
        "version": 1,
        "metadata": {"component": component(main_module, sums), "tools": {"components": [{"type": "application", "name": "Mosaid stdlib SBOM generator", "version": "1"}]}},
        "components": components,
        "dependencies": [{"ref": module_ref(main_module), "dependsOn": refs}],
    }
    output = Path(args.output)
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(document, indent=2, sort_keys=True) + "\n", encoding="utf-8")


if __name__ == "__main__":
    main()

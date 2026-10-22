#!/usr/bin/env bash
# Cross-compile the Mosaid Windows kit (no CGO, no Go needed on the target).
#
# Usage: scripts/dev/build-windows-kit.sh [output_dir]
# Produces release/windows-amd64/mosaid-windows-amd64.zip and its SHA-256.
set -euo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
out="${1:-$repo/release/windows-amd64}"
stage="$(mktemp -d)"
trap 'rm -rf "$stage"' EXIT

kit="$stage/mosaid-windows-amd64"
mkdir -p "$kit"

version="${MOSAID_VERSION:-v0.15.0}"

echo "== building windows/amd64 (version $version) =="
(cd "$repo" && CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath \
  -ldflags "-s -w -X main.version=$version" -o "$kit/mosaid.exe" ./cmd/mosaid)
(cd "$repo" && CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath \
  -ldflags "-s -w -X main.version=$version" -o "$kit/mosaid-cli.exe" ./cmd/mosaid-cli)

echo "== copying the laptop config template and guide =="
# Windows paths, not the /HOME placeholder of the generic laptop template.
python3 - "$repo/config/mosaid.laptop.example.json" "$kit/config.example.json" <<'PY'
import json, sys
source, target = sys.argv[1], sys.argv[2]
config = json.load(open(source))
config["data_dir"] = "C:/mosaid/data"
config["workspace_dir"] = "C:/mosaid/notes"
config["model"]["api_key_file"] = "C:/mosaid/secrets/model.key"
with open(target, "w", newline="\r\n") as handle:
    json.dump(config, handle, indent=2, ensure_ascii=False)
    handle.write("\n")
PY
cp "$repo/docs/agent-core/RUN-WINDOWS.ar.md" "$kit/RUN-WINDOWS.ar.md"

cat > "$kit/start-mosaid.bat" <<'BAT'
@echo off
REM Mosaid launcher for Windows 10. Edit CONFIG below if you moved the folder.
setlocal
set "HERE=%~dp0"
if "%MOSAID_CONFIG%"=="" set "MOSAID_CONFIG=%HERE%config.json"
REM Keep the Go heap small on 4 GB laptops; measured peak is ~16 MB.
if "%GOMEMLIMIT%"=="" set "GOMEMLIMIT=64MiB"
if not exist "%MOSAID_CONFIG%" (
  echo Config not found: %MOSAID_CONFIG%
  echo Copy config.example.json to config.json and put your API key file path in it.
  pause
  exit /b 2
)
"%HERE%mosaid-cli.exe" --config "%MOSAID_CONFIG%" %*
BAT
# Batch files must use CRLF line endings or cmd.exe misparses labels.
sed -i 's/$/\r/' "$kit/start-mosaid.bat"

echo "== writing checksums =="
(cd "$kit" && sha256sum mosaid.exe mosaid-cli.exe config.example.json RUN-WINDOWS.ar.md start-mosaid.bat > SHA256SUMS.txt)

echo "== packaging =="
mkdir -p "$out"
rm -f "$out/mosaid-windows-amd64.zip" "$out/mosaid-windows-amd64.zip.sha256"
(cd "$stage" && zip -q -9 -r "$out/mosaid-windows-amd64.zip" mosaid-windows-amd64)
(cd "$out" && sha256sum mosaid-windows-amd64.zip > mosaid-windows-amd64.zip.sha256)

echo "== result =="
ls -lh "$out/mosaid-windows-amd64.zip"
cat "$out/mosaid-windows-amd64.zip.sha256"

#!/usr/bin/env bash
#
# docs-config-check: assert every `yaml:` struct tag in internal/config is
# documented somewhere under docs/ (CONFIGURATION.md or a feature guide). This
# keeps the configuration reference from silently drifting behind the code.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

fail=0
count=0
while read -r key; do
  count=$((count + 1))
  if ! grep -rqiw "$key" docs/*.md; then
    echo "undocumented config key: ${key} (document it under docs/)" >&2
    fail=1
  fi
done < <(grep -rhoE 'yaml:"[a-z_]+' internal/config/*.go | sed 's/yaml:"//' | sort -u)

if [ "$fail" -ne 0 ]; then
  exit 1
fi
echo "OK: all ${count} config keys are documented under docs/."

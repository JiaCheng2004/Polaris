#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
export STACK=local
exec "${script_dir}/stack.sh" "$@"

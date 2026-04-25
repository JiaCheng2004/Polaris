#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/.." && pwd)"

usage() {
  cat <<'EOF'
Usage: scripts/stack.sh <command> [stack] [-- extra docker compose args]

Commands:
  up         Build and start the selected stack in detached mode
  down       Stop and remove the selected stack
  restart    Restart the selected stack
  logs       Follow logs for the selected stack
  ps         Show container status for the selected stack
  config     Render the effective Compose config for the selected stack
  validate   Validate the effective Compose config without rendering it
  pull       Pull images for the selected stack

Stacks:
  local      Current local runtime: Polaris + SQLite volume
  prod       Production-shaped stack: Polaris + PostgreSQL + Redis
  dev        Dev tooling stack: prod stack + Prometheus + Grafana + pgAdmin

Examples:
  ./scripts/stack.sh up local
  ./scripts/stack.sh logs dev
  STACK=prod ./scripts/stack.sh config

Environment:
  STACK                 Default stack when omitted. Defaults to local.
  COMPOSE_PROJECT_NAME  Base Compose project name. Defaults to polaris.
  POLARIS_PORT          Host port for Polaris. Defaults to 8080.

If .env exists at the repo root, it is passed to docker compose automatically.
EOF
}

require_compose() {
  if ! command -v docker >/dev/null 2>&1; then
    echo "docker is required" >&2
    exit 1
  fi
  if ! docker compose version >/dev/null 2>&1; then
    echo "docker compose is required" >&2
    exit 1
  fi
}

stack_file() {
  case "$1" in
    local)
      printf "%s/deployments/docker-compose.local.yml" "${repo_root}"
      ;;
    prod|production)
      printf "%s/deployments/docker-compose.yml" "${repo_root}"
      ;;
    dev)
      printf "%s/deployments/docker-compose.dev.yml" "${repo_root}"
      ;;
    *)
      echo "unknown stack: $1" >&2
      exit 1
      ;;
  esac
}

normalize_stack() {
  case "$1" in
    production)
      printf "prod"
      ;;
    *)
      printf "%s" "$1"
      ;;
  esac
}

compose() {
  local stack="$1"
  shift

  local compose_file
  compose_file="$(stack_file "${stack}")"

  local project_base="${COMPOSE_PROJECT_NAME:-polaris}"
  local project_name="${project_base}-$(normalize_stack "${stack}")"
  local -a args=(
    --project-directory "${repo_root}"
    --project-name "${project_name}"
    -f "${compose_file}"
  )

  if [[ -f "${repo_root}/.env" ]]; then
    args+=(--env-file "${repo_root}/.env")
  fi

  docker compose "${args[@]}" "$@"
}

main() {
  local command="${1:-}"
  shift || true

  if [[ -z "${command}" ]]; then
    usage
    exit 1
  fi

  local stack="${STACK:-local}"
  if [[ $# -gt 0 ]]; then
    case "$1" in
      local|prod|production|dev)
        stack="$1"
        shift
        ;;
    esac
  fi
  stack="$(normalize_stack "${stack}")"

  require_compose

  case "${command}" in
    up)
      compose "${stack}" up --build -d "$@"
      ;;
    down)
      compose "${stack}" down "$@"
      ;;
    restart)
      compose "${stack}" down
      compose "${stack}" up --build -d "$@"
      ;;
    logs)
      compose "${stack}" logs -f "$@"
      ;;
    ps)
      compose "${stack}" ps "$@"
      ;;
    config)
      compose "${stack}" config "$@"
      ;;
    validate)
      compose "${stack}" config --quiet "$@"
      ;;
    pull)
      compose "${stack}" pull "$@"
      ;;
    -h|--help|help)
      usage
      ;;
    *)
      echo "unknown command: ${command}" >&2
      usage
      exit 1
      ;;
  esac
}

main "$@"

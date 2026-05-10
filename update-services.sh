#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: ./update-services.sh [options]

Pull the latest code, detect changed paths, and rebuild only affected app
services: backend, webapp, miniapp. Middleware services are never targeted.

Options:
  --no-pull    Skip git pull and compare working tree against HEAD
  --force      Rebuild backend, webapp and miniapp regardless of changed paths
  --logs       Follow logs for updated services after restart
  --dry-run    Print commands without executing them
  -h, --help   Show this help
EOF
}

no_pull=0
force=0
logs=0
dry_run=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --no-pull)
      no_pull=1
      ;;
    --force)
      force=1
      ;;
    --logs)
      logs=1
      ;;
    --dry-run)
      dry_run=1
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
  shift
done

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "${repo_root}"

run() {
  printf '> '
  printf '%q ' "$@"
  printf '\n'
  if [[ "${dry_run}" == "1" ]]; then
    return 0
  fi
  "$@"
}

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "$1 command not found" >&2
    exit 1
  fi
}

add_service() {
  local service="$1"
  local existing
  for existing in "${target_services[@]}"; do
    if [[ "${existing}" == "${service}" ]]; then
      return
    fi
  done
  target_services+=("${service}")
}

require_command git
require_command docker

if [[ ! -f docker-compose.yml ]]; then
  echo "docker-compose.yml not found. Run this script from the TgLpBot repository root." >&2
  exit 1
fi

if [[ ! -f backend/.env ]]; then
  echo "backend/.env not found. Copy backend/.env.example to backend/.env and fill required values first." >&2
  exit 1
fi

before_rev="$(git rev-parse HEAD)"

if [[ "${no_pull}" != "1" ]]; then
  run git pull --ff-only
fi

after_rev="$(git rev-parse HEAD)"
changed_files=()

if [[ "${force}" == "1" ]]; then
  changed_files=(backend webapp miniapp)
elif [[ "${no_pull}" == "1" ]]; then
  mapfile -t changed_files < <(git diff --name-only HEAD -- backend webapp miniapp docker-compose.yml)
else
  if [[ "${before_rev}" == "${after_rev}" ]]; then
    changed_files=()
  else
    mapfile -t changed_files < <(git diff --name-only "${before_rev}" "${after_rev}")
  fi
fi

target_services=()

for path in "${changed_files[@]}"; do
  case "${path}" in
    backend/*|backend)
      add_service backend
      ;;
    webapp/*|webapp)
      add_service webapp
      ;;
    miniapp/*|miniapp)
      add_service miniapp
      ;;
    docker-compose.yml)
      add_service backend
      add_service webapp
      add_service miniapp
      ;;
  esac
done

if [[ ${#target_services[@]} -eq 0 ]]; then
  echo "No backend/webapp/miniapp changes detected. Nothing to rebuild."
  exit 0
fi

echo "Changed app services: ${target_services[*]}"
run docker compose up -d --build "${target_services[@]}"
run docker compose ps "${target_services[@]}"

if [[ "${logs}" == "1" ]]; then
  run docker compose logs -f "${target_services[@]}"
fi

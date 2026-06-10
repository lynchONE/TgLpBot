#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: ./update-services.sh [options]

Pull the latest code and restart the Docker Compose middleware services:
mysql, redis, minio, and minio-init.

Options:
  --no-pull    Skip git pull
  --logs       Follow middleware logs after restart
  --dry-run    Print commands without executing them
  -h, --help   Show this help
EOF
}

no_pull=0
logs=0
dry_run=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --no-pull)
      no_pull=1
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

require_command git
require_command docker

if [[ ! -f docker-compose.yml ]]; then
  echo "docker-compose.yml not found. Run this script from the TgLpBot repository root." >&2
  exit 1
fi

if [[ "${no_pull}" != "1" ]]; then
  run git pull --ff-only
fi

run docker compose up -d
run docker compose ps

if [[ "${logs}" == "1" ]]; then
  run docker compose logs -f
fi

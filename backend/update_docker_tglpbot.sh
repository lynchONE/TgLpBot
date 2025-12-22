#!/usr/bin/env bash
set -euo pipefail

# Update + restart TgLpBot Docker container.
#
# Usage:
#   ./update_docker_tglpbot.sh
#
# Optional env overrides:
#   IMAGE=lynchone/repo:tglpbot
#   CONTAINER_NAME=tglpbot
#   HOST_PORT=8080
#   CONTAINER_PORT=8080
#   ENV_FILE_HOST_PATH=/app/go/.env
#   ENV_FILE_CONTAINER_PATH=/app/.env

IMAGE="${IMAGE:-lynchone/repo:tglpbot}"
CONTAINER_NAME="${CONTAINER_NAME:-tglpbot}"
HOST_PORT="${HOST_PORT:-8080}"
CONTAINER_PORT="${CONTAINER_PORT:-8080}"
ENV_FILE_HOST_PATH="${ENV_FILE_HOST_PATH:-/app/go/.env}"
ENV_FILE_CONTAINER_PATH="${ENV_FILE_CONTAINER_PATH:-/app/.env}"

echo "[1/4] Stop + remove existing container (if any)..."
if docker ps -a --format '{{.Names}}' | grep -qx "${CONTAINER_NAME}"; then
  docker stop "${CONTAINER_NAME}" >/dev/null 2>&1 || true
  docker rm "${CONTAINER_NAME}" >/dev/null 2>&1 || true
else
  echo "  - Container ${CONTAINER_NAME} not found; skip"
fi

echo "[2/4] Pull latest image: ${IMAGE}"
docker pull "${IMAGE}"

echo "[3/4] Start new container: ${CONTAINER_NAME}"
if [ ! -f "${ENV_FILE_HOST_PATH}" ]; then
  echo "ERROR: env file not found: ${ENV_FILE_HOST_PATH}" >&2
  echo "Set ENV_FILE_HOST_PATH to your server .env path." >&2
  exit 1
fi

docker run -d \
  --name "${CONTAINER_NAME}" \
  --restart unless-stopped \
  -p "${HOST_PORT}:${CONTAINER_PORT}" \
  -v "${ENV_FILE_HOST_PATH}:${ENV_FILE_CONTAINER_PATH}:ro" \
  "${IMAGE}"

echo "[4/4] Done"
docker ps --filter "name=^/${CONTAINER_NAME}$"

#!/usr/bin/env bash
set -euo pipefail

# Update + restart TgLpBot backend, webapp and miniapp Docker containers.
#
# Usage:
#   ./update_docker_tglpbot.sh
#
# Optional env overrides:
#   NETWORK_NAME=tglpbot-network
#
#   BOT_IMAGE=lynchone/repo:tglpbot
#   BOT_CONTAINER_NAME=tglpbot
#   BOT_HOST_PORT=8080
#   BOT_CONTAINER_PORT=8080
#   ENV_FILE_HOST_PATH=/app/go/.env
#   ENV_FILE_CONTAINER_PATH=/app/.env
#
#   WEBAPP_IMAGE=lynchone/repo:tglpbot-webapp
#   WEBAPP_CONTAINER_NAME=tglpbot-webapp
#   WEBAPP_HOST_PORT=3000
#   WEBAPP_CONTAINER_PORT=80
#   WEBAPP_BACKEND_UPSTREAM=http://tglpbot:8080
#
#   MINIAPP_IMAGE=lynchone/repo:tglpbot-miniapp
#   MINIAPP_CONTAINER_NAME=tglpbot-miniapp
#   MINIAPP_HOST_PORT=3001
#   MINIAPP_CONTAINER_PORT=80
#   MINIAPP_BACKEND_UPSTREAM=http://tglpbot:8080
#
# Backward-compatible backend aliases:
#   IMAGE=lynchone/repo:tglpbot
#   CONTAINER_NAME=tglpbot
#   HOST_PORT=8080
#   CONTAINER_PORT=8080

NETWORK_NAME="${NETWORK_NAME:-tglpbot-network}"

BOT_IMAGE="${BOT_IMAGE:-${IMAGE:-lynchone/repo:tglpbot}}"
BOT_CONTAINER_NAME="${BOT_CONTAINER_NAME:-${CONTAINER_NAME:-tglpbot}}"
BOT_HOST_PORT="${BOT_HOST_PORT:-${HOST_PORT:-8080}}"
BOT_CONTAINER_PORT="${BOT_CONTAINER_PORT:-${CONTAINER_PORT:-8080}}"
ENV_FILE_HOST_PATH="${ENV_FILE_HOST_PATH:-/app/go/.env}"
ENV_FILE_CONTAINER_PATH="${ENV_FILE_CONTAINER_PATH:-/app/.env}"

WEBAPP_IMAGE="${WEBAPP_IMAGE:-lynchone/repo:tglpbot-webapp}"
WEBAPP_CONTAINER_NAME="${WEBAPP_CONTAINER_NAME:-tglpbot-webapp}"
WEBAPP_HOST_PORT="${WEBAPP_HOST_PORT:-3000}"
WEBAPP_CONTAINER_PORT="${WEBAPP_CONTAINER_PORT:-80}"
WEBAPP_BACKEND_UPSTREAM="${WEBAPP_BACKEND_UPSTREAM:-http://${BOT_CONTAINER_NAME}:${BOT_CONTAINER_PORT}}"

MINIAPP_IMAGE="${MINIAPP_IMAGE:-lynchone/repo:tglpbot-miniapp}"
MINIAPP_CONTAINER_NAME="${MINIAPP_CONTAINER_NAME:-tglpbot-miniapp}"
MINIAPP_HOST_PORT="${MINIAPP_HOST_PORT:-3001}"
MINIAPP_CONTAINER_PORT="${MINIAPP_CONTAINER_PORT:-80}"
MINIAPP_BACKEND_UPSTREAM="${MINIAPP_BACKEND_UPSTREAM:-http://${BOT_CONTAINER_NAME}:${BOT_CONTAINER_PORT}}"

ensure_network() {
  if docker network inspect "${NETWORK_NAME}" >/dev/null 2>&1; then
    return
  fi
  docker network create "${NETWORK_NAME}" >/dev/null
}

pull_image() {
  local image="$1"
  echo "Pull image: ${image}"
  docker pull "${image}"
}

remove_container() {
  local name="$1"
  if docker ps -a --format '{{.Names}}' | grep -qx "${name}"; then
    docker stop "${name}" >/dev/null 2>&1 || true
    docker rm "${name}" >/dev/null 2>&1 || true
  else
    echo "Container ${name} not found; skip"
  fi
}

start_bot() {
  if [ ! -f "${ENV_FILE_HOST_PATH}" ]; then
    echo "ERROR: env file not found: ${ENV_FILE_HOST_PATH}" >&2
    echo "Set ENV_FILE_HOST_PATH to your server .env path." >&2
    exit 1
  fi

  docker run -d \
    --name "${BOT_CONTAINER_NAME}" \
    --restart unless-stopped \
    --network "${NETWORK_NAME}" \
    -p "${BOT_HOST_PORT}:${BOT_CONTAINER_PORT}" \
    -v "${ENV_FILE_HOST_PATH}:${ENV_FILE_CONTAINER_PATH}:ro" \
    "${BOT_IMAGE}"
}

start_frontend() {
  local name="$1"
  local image="$2"
  local host_port="$3"
  local container_port="$4"
  local backend_upstream="$5"

  docker run -d \
    --name "${name}" \
    --restart unless-stopped \
    --network "${NETWORK_NAME}" \
    -p "${host_port}:${container_port}" \
    -e "BACKEND_UPSTREAM=${backend_upstream}" \
    "${image}"
}

echo "[1/5] Ensure Docker network: ${NETWORK_NAME}"
ensure_network

echo "[2/5] Pull latest images"
pull_image "${BOT_IMAGE}"
pull_image "${WEBAPP_IMAGE}"
pull_image "${MINIAPP_IMAGE}"

echo "[3/5] Stop + remove existing containers"
remove_container "${WEBAPP_CONTAINER_NAME}"
remove_container "${MINIAPP_CONTAINER_NAME}"
remove_container "${BOT_CONTAINER_NAME}"

echo "[4/5] Start containers"
start_bot
start_frontend "${WEBAPP_CONTAINER_NAME}" "${WEBAPP_IMAGE}" "${WEBAPP_HOST_PORT}" "${WEBAPP_CONTAINER_PORT}" "${WEBAPP_BACKEND_UPSTREAM}"
start_frontend "${MINIAPP_CONTAINER_NAME}" "${MINIAPP_IMAGE}" "${MINIAPP_HOST_PORT}" "${MINIAPP_CONTAINER_PORT}" "${MINIAPP_BACKEND_UPSTREAM}"

echo "[5/5] Done"
docker ps --filter "name=^/${BOT_CONTAINER_NAME}$"
docker ps --filter "name=^/${WEBAPP_CONTAINER_NAME}$"
docker ps --filter "name=^/${MINIAPP_CONTAINER_NAME}$"

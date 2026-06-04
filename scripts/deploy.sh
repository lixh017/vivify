#!/usr/bin/env bash
#
# scripts/deploy.sh — build the OPC images locally, ship them to a remote
# VPS over SSH, and restart the production stack with docker compose.
#
# Required tooling on the deploy host:
#   - docker (with the compose plugin: `docker compose`)
#   - ssh + scp
#   - (optional) .env.production in the repo root for secrets
#
# Environment overrides:
#   VPS_HOST        SSH target / SSH config alias (default: opc-vps)
#   REMOTE_DIR      Remote deployment directory (default: /opt/opc)
#   SSH_OPTS        Extra ssh options (default: none)
#   PUSH_REGISTRY   If set, push images to this registry instead of
#                   docker save/load. The remote host must already be
#                   logged in / able to pull from $PUSH_REGISTRY.
#                   Example: ghcr.io/your-org/opc
#   SKIP_BUILD=1    Skip local `docker compose build` (use pre-built images)
#   SKIP_HEALTH=1   Skip the post-deploy health check
#
set -euo pipefail

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------
VPS_HOST="${VPS_HOST:-opc-vps}"
REMOTE_DIR="${REMOTE_DIR:-/opt/opc}"
SSH_OPTS="${SSH_OPTS:-}"
PUSH_REGISTRY="${PUSH_REGISTRY:-}"
SKIP_BUILD="${SKIP_BUILD:-}"
SKIP_HEALTH="${SKIP_HEALTH:-}"

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

# Image names — must match the `image:` keys in docker-compose.yml.
API_IMAGE="opc-api"
WEB_IMAGE="opc-web"
COMPOSE_FILE="docker-compose.yml"

# ---------------------------------------------------------------------------
# Logging helpers
# ---------------------------------------------------------------------------
log()  { printf '\033[1;34m[deploy]\033[0m %s\n' "$*"; }
ok()   { printf '\033[1;32m[deploy]\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[deploy]\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31m[deploy]\033[0m %s\n' "$*" >&2; exit 1; }

# ---------------------------------------------------------------------------
# Preflight
# ---------------------------------------------------------------------------
need_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "Required command not found: $1"
}

log "Preflight checks..."
need_cmd docker
need_cmd ssh
need_cmd scp

if ! docker compose version >/dev/null 2>&1; then
  die "docker compose plugin not available — install Compose v2"
fi

# ---------------------------------------------------------------------------
# Build
# ---------------------------------------------------------------------------
if [[ -z "$SKIP_BUILD" ]]; then
  log "Building images locally..."
  docker compose -f "$COMPOSE_FILE" build
else
  log "SKIP_BUILD=1 — skipping local build"
fi

# ---------------------------------------------------------------------------
# Ship images
# ---------------------------------------------------------------------------
ship_image_save_load() {
  local image="$1"
  log "Streaming $image to $VPS_HOST via docker save | docker load..."
  # Use a deterministic tag based on the current timestamp so a rollback is
  # possible: `docker tag opc-api:<ts> opc-api:current` is run on the remote.
  local tag
  tag="$(date -u +%Y%m%d%H%M%S)"

  docker save "$image:latest" | ssh $SSH_OPTS "$VPS_HOST" \
    "docker load && docker tag $image:latest $image:$tag && \
     docker tag $image:latest $image:current"
}

ship_image_registry() {
  local image="$1"
  local registry="$PUSH_REGISTRY"
  log "Tagging & pushing $image to $registry..."
  docker tag "$image:latest" "$registry/$image:latest"
  docker push "$registry/$image:latest"
}

for img in "$API_IMAGE" "$WEB_IMAGE"; do
  if [[ -n "$PUSH_REGISTRY" ]]; then
    ship_image_registry "$img"
  else
    ship_image_save_load "$img"
  fi
done

# ---------------------------------------------------------------------------
# Ship compose file (+ .env.production if present)
# ---------------------------------------------------------------------------
log "Copying $COMPOSE_FILE to $VPS_HOST:$REMOTE_DIR/"
ssh $SSH_OPTS "$VPS_HOST" "mkdir -p '$REMOTE_DIR'"
scp $SSH_OPTS "$COMPOSE_FILE" "$VPS_HOST:$REMOTE_DIR/"

if [[ -f .env.production ]]; then
  log "Copying .env.production to $VPS_HOST:$REMOTE_DIR/"
  scp $SSH_OPTS .env.production "$VPS_HOST:$REMOTE_DIR/"
fi

# ---------------------------------------------------------------------------
# Remote restart
# ---------------------------------------------------------------------------
log "Restarting stack on $VPS_HOST..."
ssh $SSH_OPTS "$VPS_HOST" "cd '$REMOTE_DIR' && \
  docker compose -f $COMPOSE_FILE down && \
  docker compose -f $COMPOSE_FILE up -d"

# ---------------------------------------------------------------------------
# Health check
# ---------------------------------------------------------------------------
if [[ -z "$SKIP_HEALTH" ]]; then
  log "Waiting for API health endpoint..."
  HEALTH_OK=0
  for attempt in $(seq 1 12); do
    if ssh $SSH_OPTS "$VPS_HOST" \
         "curl -fsS --max-time 5 http://localhost:8080/health" >/dev/null 2>&1; then
      HEALTH_OK=1
      break
    fi
    warn "  health check attempt $attempt/12 failed, retrying in 5s..."
    sleep 5
  done

  if [[ "$HEALTH_OK" -eq 1 ]]; then
    ok "API is healthy"
  else
    warn "API did not become healthy within 60s"
    warn "Inspect with: ssh $VPS_HOST 'cd $REMOTE_DIR && docker compose ps'"
    exit 1
  fi
else
  log "SKIP_HEALTH=1 — skipping post-deploy health check"
fi

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
ok "Deployed to $VPS_HOST"
log "Useful follow-up commands:"
cat <<EOF
  ssh $VPS_HOST 'cd $REMOTE_DIR && docker compose ps'
  ssh $VPS_HOST 'cd $REMOTE_DIR && docker compose logs -f --tail=100'
  ssh $VPS_HOST 'curl -s http://localhost:8080/health'
EOF

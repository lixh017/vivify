#!/bin/bash
set -e

case "$1" in
  setup)
    echo "📦 Setting up OPC monorepo..."
    # Idempotent: only initialize modules if not already present.
    if [ ! -f apps/api/go.mod ]; then
      (cd apps/api && go mod init github.com/opc/api)
    else
      echo "  ↳ apps/api/go.mod already exists, skipping go mod init"
    fi
    if [ ! -f apps/web/package.json ]; then
      (cd apps/web && npm init -y)
    else
      echo "  ↳ apps/web/package.json already exists, skipping npm init"
    fi
    # Always run dependency resolution (cheap if up to date).
    (cd apps/api && go mod download)
    if command -v pnpm >/dev/null 2>&1; then
      (cd apps/web && pnpm install)
    else
      (cd apps/web && npm install)
    fi
    mkdir -p apps/web/app apps/api/internal
    echo "✅ Setup complete"
    ;;
  up)
    echo "❌ No docker-compose.yml in repo root."
    echo "   Use 'make build' and 'cd apps/api && go run ./cmd/server' instead."
    exit 1
    ;;
  down)
    echo "❌ No docker-compose.yml in repo root; nothing to stop."
    exit 1
    ;;
  *)
    echo "Usage: $0 {setup|up|down}"
    exit 1
    ;;
esac

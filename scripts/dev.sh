#!/bin/bash
set -e

case "$1" in
  setup)
    echo "📦 Setting up OPC monorepo..."
    cd apps/api && go mod init github.com/opc/api && cd ../..
    cd apps/web && npm init -y && cd ../..
    mkdir -p apps/web/app apps/api/internal
    echo "✅ Setup complete"
    ;;
  up)
    echo "🚀 Starting dev environment..."
    docker compose up -d
    echo "✅ API: http://localhost:8080, Web: http://localhost:3000"
    ;;
  down)
    docker compose down
    ;;
  *)
    echo "Usage: $0 {setup|up|down}"
    exit 1
    ;;
esac

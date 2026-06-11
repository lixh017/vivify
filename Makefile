# OPC monorepo Makefile.
#
# The Go API requires the `fts5` build tag at compile time AND at test
# time (mattn/go-sqlite3 only ships the FTS5 module when the tag is
# set). All `make test` and `make build` targets therefore pass the tag
# explicitly so the documented review command (`go test -race ./...`)
# can be reproduced by running `make test` instead of having to
# remember the build flag.

API_DIR      := apps/api
FTS5_TAG     := fts5
GO           := go
GO_TEST      := $(GO) test -race -tags $(FTS5_TAG)
GO_BUILD     := $(GO) build -tags $(FTS5_TAG)
PKG          := ./...

.PHONY: help test test-integration test-plain build smoke-api lint fmt vet clean stop stop-8081 bench asset asset-build asset-check asset-generate-image

help:
	@echo "OPC make targets:"
	@echo "  test             Run API test suite (race + fts5)"
	@echo "  test-integration Run real Claude API integration tests (gated by ANTHROPIC_API_KEY)"
	@echo "  test-plain       Run API tests WITHOUT -tags fts5 (smoke; FTS integration tests are skipped)"
	@echo "  build            Build the API server binary (with fts5 tag)"
	@echo "  asset            Build the opc-asset CLI (brand-on asset generator, see apps/api/cmd/opc-asset)"
	@echo "  asset-check      Score a prompt via the brand profile (PROMPT=...)"
	@echo "  asset-generate-image  Generate one image with default scene+outfit"
	@echo "  bench            Run API benchmarks (FTS search)"
	@echo "  smoke-api        Build the binary and run the 5-handler E2E smoke (scripts/api-smoke.sh)"
	@echo "  fmt              gofmt -w all .go files under apps/api"
	@echo "  vet              go vet -tags fts5 ./..."
	@echo "  clean            Remove build artifacts"
	@echo "  stop             Kill the API server listener on :8080 (real binary, not the go run wrapper)"
	@echo "  stop-8081        Kill the API server listener on :8081"

# Full test suite with race detector and FTS5 enabled.
test:
	cd $(API_DIR) && $(GO_TEST) $(PKG)

# Plain `go test ./...` — verifies the FTS-gated tests are skipped
# (not failed) when the build tag is absent. The CRUD and unit tests
# still run.
test-plain:
	cd $(API_DIR) && $(GO) test -race $(PKG)

# Real Claude API integration tests. Gated by the `integration` build
# tag AND the ANTHROPIC_API_KEY env var. When the key is missing we
# skip with a clear message — never fail — so a developer with no key
# can still run `make test-integration` and see the regular suite
# pass with the integration tests Skip-not-Fail. The suite uses
# Claude Haiku 4.5 by default to keep cost well under a cent.
# Example: ANTHROPIC_API_KEY=sk-ant-... make test-integration
test-integration:
	@if [ -n "$$ANTHROPIC_API_KEY" ]; then \
		cd $(API_DIR) && $(GO) test -tags integration -race $(PKG); \
	else \
		echo 'ANTHROPIC_API_KEY not set; skipping integration tests'; \
	fi

build:
	cd $(API_DIR) && $(GO_BUILD) -trimpath -ldflags="-s -w" -o ../bin/opc-api ./cmd/server

# opc-asset — the brand-on asset CLI. Built with the FTS5 tag for
# parity with the API server (the CLI doesn't use SQLite but
# sharing the tag keeps the build command line simple and the
# future option to persist ledger entries to a DB open).
# Output: apps/bin/opc-asset
asset-build:
	cd $(API_DIR) && $(GO_BUILD) -trimpath -ldflags="-s -w" -o ../bin/opc-asset ./cmd/opc-asset

# Convenience: run an IP-consistency check on a prompt. Useful for
# validating a hand-written prompt before sending it to a model.
# Example: make asset-check PROMPT='peakge in bamboo'
asset-check: asset-build
	./bin/opc-asset check --prompt '$(PROMPT)'

# Convenience: generate one image with default scene (竹林小院)
# and outfit (朱红). Output: apps/assets/manual.jpg
asset-generate-image: asset-build
	./bin/opc-asset generate --type image --scene-id manual --out assets/manual.jpg

# Back-compat alias
asset: asset-build

bench:
	cd $(API_DIR) && $(GO_TEST) -bench=. -benchmem -run=^$$ $(PKG)

# Full 5-handler E2E regression: boots a fresh API on :8081 against
# a throwaway SQLite DB, runs cover/humanize/score/postmortem/pipeline,
# prints a summary table, and tears the server down on exit.
# See scripts/api-smoke.sh for env overrides (PORT, DB_PATH, etc.).
smoke-api: build
	./scripts/api-smoke.sh

fmt:
	cd $(API_DIR) && $(GO) fmt ./...

vet:
	cd $(API_DIR) && $(GO) vet -tags $(FTS5_TAG) $(PKG)

clean:
	rm -rf $(API_DIR)/bin

stop:
	./scripts/stop-server.sh 8080

stop-8081:
	./scripts/stop-server.sh 8081

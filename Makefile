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

.PHONY: help test test-plain build smoke lint fmt vet clean bench

help:
	@echo "OPC make targets:"
	@echo "  test         Run API test suite (race + fts5)"
	@echo "  test-plain   Run API tests WITHOUT -tags fts5 (smoke; FTS integration tests are skipped)"
	@echo "  build        Build the API server binary (with fts5 tag)"
	@echo "  bench        Run API benchmarks (FTS search)"
	@echo "  smoke        Build the binary and run healthcheck"
	@echo "  fmt          gofmt -w all .go files under apps/api"
	@echo "  vet          go vet -tags fts5 ./..."
	@echo "  clean        Remove build artifacts"

# Full test suite with race detector and FTS5 enabled.
test:
	cd $(API_DIR) && $(GO_TEST) $(PKG)

# Plain `go test ./...` — verifies the FTS-gated tests are skipped
# (not failed) when the build tag is absent. The CRUD and unit tests
# still run.
test-plain:
	cd $(API_DIR) && $(GO) test -race $(PKG)

build:
	cd $(API_DIR) && $(GO_BUILD) -trimpath -ldflags="-s -w" -o ../bin/opc-api ./cmd/server

bench:
	cd $(API_DIR) && $(GO_TEST) -bench=. -benchmem -run=^$$ $(PKG)

smoke: build
	./bin/opc-api --help 2>&1 | head -1 || true

fmt:
	cd $(API_DIR) && $(GO) fmt ./...

vet:
	cd $(API_DIR) && $(GO) vet -tags $(FTS5_TAG) $(PKG)

clean:
	rm -rf $(API_DIR)/bin

// opc-mcp — the standalone OPC MCP stdio binary.
//
// This binary exists separately from the HTTP API (opc-api) so
// that MCP-aware AI clients (Claude Code, Cursor, etc.) can
// spawn a dedicated process per session without paying for the
// HTTP server's goroutine + Gin router + DB connection pool.
//
// Wire shape:
//
//   ┌─ 用户的 mac ──────────────────────────────────────┐
//   │                                                    │
//   │  Claude Code 进程                                   │
//   │    ↕ spawns + JSON-RPC over stdio (the MCP protocol) │
//   │  opc-mcp binary (本文件, 30MB)                       │
//   │    ↕ 直接读 DB_PATH=/tmp/...                        │
//   │  SQLite 文件                                          │
//   │                                                    │
//   └────────────────────────────────────────────────────┘
//
// Compare with the previous embedding in opc-api:
//
//   ┌─ 用户的 mac ──────────────────────────────────────┐
//   │                                                    │
//   │  Claude Code 进程                                   │
//   │    ✗ 不知道 opc-mcp 怎么启                          │
//   │                                                    │
//   │  opc-api binary (HTTP + stdio MCP 嵌一起)             │
//   │    - HTTP :8080 (Gin router, 长期进程)              │
//   │    - stdio MCP 在 goroutine (用不到 — 同一进程的        │
//   │      HTTP server 跟 MCP client 不能共享 stdin)         │
//   │    - 30MB RAM + HTTP port + 一个长期 HTTP process    │
//   │                                                    │
//   └────────────────────────────────────────────────────┘
//
// Transport is stdio JSON-RPC (the MCP reference transport; see
// spec.modelcontextprotocol.io). HTTP+SSE / Streamable HTTP
// is the formal alternative for remote servers; OPC does not
// need it yet (single-machine, single-team, single-panda-IP).
// The HTTP API remains the right surface for web UI + third-party
// REST clients — the two surfaces serve different clients
// (GitHub / Linear / Notion all do exactly this).
//
// Configuration is via env, mirroring the rest of the OPC
// monorepo so the same .env or docker-compose file works for
// both binaries:
//
//   DB_PATH          SQLite path (default: ./opc.db)
//   MINIMAX_API_KEY  upstream LLM key (default: empty → demo
//                    canned responses; see agents.MiniMax)
//   ENCRYPTION_KEY   32-byte base64 key for credential
//                    decryption. Required for any production
//                    deployment with configured credentials.
//                    Without it the resolver still works for
//                    default-text routing but credential
//                    decryption fails (the resolver logs a
//                    clear error per call).
//
// Invocation:
//
//   $ ./opc-mcp
//   # blocks on stdin; client sends JSON-RPC requests and
//   # receives JSON-RPC responses on stdout. Exit on EOF or
//   # SIGTERM/SIGINT (Ctrl-C in foreground).
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/opc/api/internal/agents"
	"github.com/opc/api/internal/db"
	"github.com/opc/api/internal/mcp"
)

func main() {
	// Structured logging to stderr (stdout is reserved for the
	// MCP JSON-RPC stream — mixing log lines into it would
	// corrupt the protocol). slog default is text format;
	// production deployments can flip via LOG_FORMAT=json
	// (mirrored from the HTTP server's setupLogger).
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "./opc.db"
	}
	apiKey := os.Getenv("MINIMAX_API_KEY")

	// Wire the same shape as the embedded server in
	// cmd/server/main.go: GORM DB + MiniMax text provider +
	// nil resolver. The resolver is a Phase 4 follow-up
	// (per-user credential routing); the standalone binary
	// keeps the simpler "default text for everyone" path
	// until content lead feedback says otherwise.
	gormDB, err := db.Connect(dbPath)
	if err != nil {
		slog.Error("db connect failed", "error", err, "path", dbPath)
		os.Exit(1)
	}
	// Run migrations on startup, same as opc-api (cmd/server/main.go).
	// Without this, a fresh DB_PATH (e.g. /tmp/opc-content.db on a new
	// install) has no tables and every opc_create_* call fails with
	// "no such table: topics". Idempotent: safe on existing DBs.
	if err := db.Migrate(gormDB); err != nil {
		slog.Error("db migrate failed", "error", err)
		os.Exit(1)
	}
	mmxClient := agents.NewMiniMax(apiKey)
	srv, err := mcp.NewServer(gormDB, mmxClient, nil)
	if err != nil {
		slog.Error("mcp server init failed", "error", err)
		os.Exit(1)
	}
	slog.Info("opc-mcp ready",
		"db", dbPath,
		"tools", srv.ListTools(),
		"transport", "stdio",
	)

	// Honor SIGINT/SIGTERM so the parent (Claude Code) can
	// shut us down cleanly. Without this, killing the parent
	// process leaves us orphaned holding the DB lock.
	ctx, cancel := signal.NotifyContext(context.Background(),
		syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := srv.ServeStdio(ctx); err != nil {
		// ServeStdio returns an error on EOF (client
		// disconnected) or on a transport failure. Both
		// are normal shutdown paths — log at info, not
		// error, so log monitoring does not alert on
		// clean exits.
		if ctx.Err() != nil {
			slog.Info("opc-mcp shut down on signal")
		} else {
			slog.Info("opc-mcp stopped", "reason", err)
		}
	}
}

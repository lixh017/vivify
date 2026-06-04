package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"github.com/opc/api/internal/agents"
	"github.com/opc/api/internal/config"
	"github.com/opc/api/internal/db"
	"github.com/opc/api/internal/handlers"
	"github.com/opc/api/internal/mcp"
	"github.com/opc/api/internal/middleware"
)

func main() {
	cfg := config.Load()

	// Route all log output through slog so request logs, MCP logs and any
	// future structured events share the same handler. We default to JSON
	// in production-like environments (LOG_FORMAT=json) and text
	// otherwise — keeping the human-friendly dev output the team is used
	// to while making it trivial to ship structured logs to a log
	// aggregator.
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)
	if os.Getenv("LOG_FORMAT") != "json" {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})))
	}

	gormDB, err := db.Connect(cfg.DBPath)
	if err != nil {
		slog.Error("db connect failed", "error", err)
		os.Exit(1)
	}

	if err := db.Migrate(gormDB); err != nil {
		slog.Error("migrate failed", "error", err)
		os.Exit(1)
	}

	// Initialize the Claude agent. An empty API key is acceptable
	// during development; Complete() will surface a clear error at
	// call time.
	claudeAgent := agents.NewClaude(cfg.AnthropicAPIKey)

	// Construct the MCP server. The stdio transport blocks for the
	// lifetime of the process, so we run it in its own goroutine and
	// let SIGINT/SIGTERM cancel the context to shut it down cleanly.
	mcpServer, err := mcp.NewServer(gormDB, claudeAgent)
	if err != nil {
		slog.Error("mcp server init failed", "error", err)
		os.Exit(1)
	}
	slog.Info("mcp tools exposed", "tools", mcpServer.ListTools())

	mcpCtx, mcpCancel := context.WithCancel(context.Background())
	defer mcpCancel()
	go func() {
		if err := mcpServer.ServeStdio(mcpCtx); err != nil {
			slog.Error("mcp stdio exited", "error", err)
			mcpCancel()
		}
	}()

	// Silence gin's default debug logger — every request now flows
	// through the structured request logger middleware below, so the
	// default writer would only duplicate output.
	gin.DefaultWriter = io.Discard
	gin.DefaultErrorWriter = io.Discard
	gin.SetMode(gin.ReleaseMode)

	r := gin.New()
	r.Use(handlers.RequestID())
	r.Use(middleware.RequestLogger())
	r.Use(gin.Recovery())
	r.Use(cors.Default())

	// /healthz is a liveness probe — process is up.
	r.GET("/healthz", handlers.Health)
	// /readyz is a readiness probe — process is up AND the DB is
	// reachable. Kubernetes-style orchestrators should gate traffic
	// on /readyz and restart on /healthz failures.
	r.GET("/readyz", handlers.NewReadyz(gormDB))

	topicH := handlers.NewTopicHandlerFromGorm(gormDB, nil)
	topicH.RegisterRoutes(r)

	scriptH := handlers.NewScriptHandler(gormDB)
	scriptH.RegisterRoutes(r)

	contentItemH := handlers.NewContentItemHandler(gormDB)
	contentItemH.RegisterRoutes(r)

	// FTS5 search route must be registered on the same router BEFORE
	// the CRUD :id route. Gin's radix tree resolves the static segment
	// "search" before the :id wildcard in practice, but we still keep
	// the registration order explicit (and the tests assert it) so a
	// future refactor that changes the URL shape — or relies on
	// Gin's tie-breaking behaviour — does not silently route search
	// traffic to the CRUD GET-by-id handler.
	knowledgeSearchH := handlers.NewKnowledgeSearchHandler(gormDB)
	knowledgeSearchH.RegisterRoutes(r)

	knowledgeDocH := handlers.NewKnowledgeDocHandler(gormDB)
	knowledgeDocH.RegisterRoutes(r)

	seriesH := handlers.NewSeriesHandler(gormDB)
	seriesH.RegisterRoutes(r)

	aiH := handlers.NewAIHandler(claudeAgent, gormDB)
	aiH.RegisterRoutes(r)

	httpSrv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		slog.Info("opc api listening", "port", cfg.Port)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	// Block on SIGINT / SIGTERM, then drain HTTP, then cancel MCP.
	// The shutdown context bounds how long we wait for in-flight
	// requests — beyond that we force-exit so a stuck handler can't
	// block the process forever.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		slog.Error("http server failed", "error", err)
		os.Exit(1)
	case s := <-sig:
		slog.Info("shutdown: signal received", "signal", s.String())
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		slog.Error("http shutdown error", "error", err)
	} else {
		slog.Info("http server stopped cleanly")
	}

	// Stop the MCP transport last so any tool calls in flight on the
	// HTTP side finish first.
	mcpCancel()
	// Give the MCP goroutine a brief window to flush — ServeStdio
	// owns its own context cancellation handling.
	mcpDone := make(chan struct{})
	go func() {
		// We don't have a WaitGroup handle for the MCP goroutine, so
		// we just wait a short bounded duration. The transport exits
		// on context cancel; this prevents a tail-of-log race where
		// the process exits before slog flushes the last record.
		time.Sleep(100 * time.Millisecond)
		close(mcpDone)
	}()
	select {
	case <-mcpDone:
	case <-time.After(2 * time.Second):
		slog.Warn("mcp shutdown timed out")
	}

	slog.Info("shutdown complete")
}

package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"github.com/opc/api/internal/agents"
	"github.com/opc/api/internal/config"
	"github.com/opc/api/internal/db"
	"github.com/opc/api/internal/handlers"
	"github.com/opc/api/internal/mcp"
)

func main() {
	cfg := config.Load()

	gormDB, err := db.Connect(cfg.DBPath)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}

	if err := db.Migrate(gormDB); err != nil {
		log.Fatalf("migrate: %v", err)
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
		log.Fatalf("mcp server: %v", err)
	}
	log.Printf("🔌 MCP tools exposed: %v", mcpServer.ListTools())

	mcpCtx, mcpCancel := context.WithCancel(context.Background())
	defer mcpCancel()
	go func() {
		if err := mcpServer.ServeStdio(mcpCtx); err != nil {
			log.Printf("mcp stdio: %v", err)
			mcpCancel()
		}
	}()

	r := gin.Default()
	r.Use(handlers.RequestID())
	r.Use(cors.Default())

	r.GET("/health", handlers.Health)

	topicH := handlers.NewTopicHandlerFromGorm(gormDB, nil)
	topicH.RegisterRoutes(r)

	scriptH := handlers.NewScriptHandler(gormDB)
	scriptH.RegisterRoutes(r)

	contentItemH := handlers.NewContentItemHandler(gormDB)
	contentItemH.RegisterRoutes(r)

	knowledgeDocH := handlers.NewKnowledgeDocHandler(gormDB)
	knowledgeDocH.RegisterRoutes(r)

	// FTS5 search route must be registered on the same router BEFORE
	// the CRUD :id route so Gin's trie resolves /knowledge/search to
	// the search handler rather than treating "search" as :id.
	knowledgeSearchH := handlers.NewKnowledgeSearchHandler(gormDB)
	knowledgeSearchH.RegisterRoutes(r)

	seriesH := handlers.NewSeriesHandler(gormDB)
	seriesH.RegisterRoutes(r)

	aiH := handlers.NewAIHandler(claudeAgent, gormDB)
	aiH.RegisterRoutes(r)

	httpSrv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: r,
	}
	go func() {
		log.Printf("🚀 OPC API listening on :%s", cfg.Port)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()

	// Block on SIGINT / SIGTERM, then cancel MCP, then drain HTTP.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Printf("shutdown: signal received")
	mcpCancel()
	_ = httpSrv.Shutdown(context.Background())
}

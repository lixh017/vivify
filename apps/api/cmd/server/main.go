package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"

	"github.com/opc/api/internal/agents"
	"github.com/opc/api/internal/auth"
	"github.com/opc/api/internal/config"
	"github.com/opc/api/internal/db"
	"github.com/opc/api/internal/handlers"
	"github.com/opc/api/internal/mcp"
	"github.com/opc/api/internal/middleware"
)

func main() {
	// Load .env from the current working directory (and parents) before
	// any other config step. Silent on miss: prod / CI typically run
	// without a .env file and inject real secrets via the process env,
	// so a missing file is not an error. Existing process env wins
	// (godotenv.Load does NOT override), so an explicit `export
	// FOO=bar` always beats the .env value.
	_ = godotenv.Load()

	cfg := config.Load()

	// Route all log output through slog so request logs, MCP logs and any
	// future structured events share the same handler. We default to JSON
	// in production-like environments (LOG_FORMAT=json) and text
	// otherwise — keeping the human-friendly dev output the team is used
	// to while making it trivial to ship structured logs to a log
	// aggregator. The branch is picked once up front so we don't set the
	// default twice.
	setupLogger(os.Getenv("LOG_FORMAT"))

	gormDB, err := db.Connect(cfg.DBPath)
	if err != nil {
		slog.Error("db connect failed", "error", err)
		os.Exit(1)
	}

	if err := db.Migrate(gormDB); err != nil {
		slog.Error("migrate failed", "error", err)
		os.Exit(1)
	}

	// Initialize the MiniMax client (single provider for text +
	// image + speech + video post-Phase-4). An empty API key is
	// acceptable during development; Available() returns false and
	// every handler short-circuits to demo data.
	mmxClient := agents.NewMiniMax(os.Getenv("MINIMAX_API_KEY"))

	// Multi-model router: kept for /readyz + the multi-model
	// observability surface. MiniMax is wired as a Provider so
	// the existing router-aware call sites still work; the
	// per-handler text client is a separate *agents.MiniMax
	// instance because the handlers consume the rich Text/Image
	// surface that the generic Provider does not expose.
	overrides, overrideWarnings := agents.ParseOverrides(cfg.ModelOverrides)
	for _, w := range overrideWarnings {
		slog.Warn("ai_model_overrides", "warning", w)
	}
	aiRouter := agents.NewRouterWithOverrides(
		[]agents.Provider{
			agents.NewDeepSeekProvider(cfg.DeepSeekAPIKey),
			agents.NewGeminiProvider(cfg.GeminiAPIKey),
		},
		overrides,
	)
	for _, p := range aiRouter.Providers() {
		slog.Info("ai_provider_registered", "name", p.Name(), "available", p.Available())
	}
	// _ = aiRouter keeps the router live for /readyz and the
	// router-aware handler migration coming in the next phase.
	_ = aiRouter

	// Construct the MCP server. The stdio transport blocks for the
	// lifetime of the process, so we run it in its own goroutine and
	// let SIGINT/SIGTERM cancel the context to shut it down cleanly.
	mcpServer, err := mcp.NewServer(gormDB, mmxClient)
	if err != nil {
		slog.Error("mcp server init failed", "error", err)
		os.Exit(1)
	}
	slog.Info("mcp tools exposed", "tools", mcpServer.ListTools())

	mcpCtx, mcpCancel := context.WithCancel(context.Background())
	defer mcpCancel()
	// Use a WaitGroup to wait for the MCP goroutine to actually
	// return — sleeping for a fixed duration is racy and can either
	// be too long (delaying process exit) or too short (letting the
	// process exit before the transport flushes its last log line).
	var mcpWG sync.WaitGroup
	mcpWG.Add(1)
	go func() {
		defer mcpWG.Done()
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
	// Metrics middleware runs before Recovery so a panic still
	// increments the request counter (and the latency histogram
	// observes the time-to-panic). /metrics itself is excluded by
	// the middleware to avoid observing scrapes.
	r.Use(middleware.Metrics())
	r.Use(gin.Recovery())
	// CORS is configured for cookie auth: AllowCredentials: true + a
	// concrete Allow-Origin (not "*"). The list resolves
	// NEXT_PUBLIC_API_URL with a loopback dev fallback — see
	// internal/middleware/cors.go.
	r.Use(middleware.CORS())

	// /metrics exposes Prometheus text format. Mounted first so it
	// never flows through the CORS/auth layers (and so a future
	// rewrite rule can't accidentally funnel scrape traffic
	// through a JSON API). promhttp's default handler uses the
	// package-level gatherer, which already includes the counters
	// and histograms registered in internal/middleware.
	r.GET("/metrics", handlers.MetricsHandler())

	// /healthz is a liveness probe — process is up.
	r.GET("/healthz", handlers.Health)
	// /readyz is a readiness probe — process is up AND the DB is
	// reachable. Kubernetes-style orchestrators should gate traffic
	// on /readyz and restart on /healthz failures. We resolve the
	// underlying *sql.DB once at startup so the readiness handler
	// only depends on the minimal Pinger interface.
	sqlDB, err := gormDB.DB()
	if err != nil {
		slog.Error("resolve sql.DB for readyz failed", "error", err)
		os.Exit(1)
	}
	r.GET("/readyz", handlers.NewReadyz(sqlDB))

	// Catch-all 404 handler. Without this, gin returns a plain-text
	// "404 page not found" body, which makes every client that calls
	// response.json() throw a SyntaxError. Match the rest of the API
	// envelope so callers can rely on a single error shape.
	r.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{
			"error":  "route not found",
			"path":   c.Request.URL.Path,
			"method": c.Request.Method,
		})
	})

	topicH := handlers.NewTopicHandlerFromGorm(gormDB, nil)
	scriptH := handlers.NewScriptHandler(gormDB)
	contentItemH := handlers.NewContentItemHandler(gormDB)

	// AES master key for the credential vault. The constructor
	// panics on a short key so a misconfigured env surfaces as a
	// startup failure rather than a 500 on the first request.
	// ENCRYPTION_KEY is read by the auth package — see
	// internal/auth/crypto.go.
	encryptionKey := auth.MustLoadKey()
	credentialH := handlers.NewCredentialHandler(gormDB, encryptionKey, slog.Default())

	// FTS5 search route must be registered on the same router BEFORE
	// the CRUD :id route. Gin's radix tree resolves the static segment
	// "search" before the :id wildcard in practice, but we still keep
	// the registration order explicit (and the tests assert it) so a
	// future refactor that changes the URL shape — or relies on
	// Gin's tie-breaking behaviour — does not silently route search
	// traffic to the CRUD GET-by-id handler.
	knowledgeSearchH := handlers.NewKnowledgeSearchHandler(gormDB)
	knowledgeDocH := handlers.NewKnowledgeDocHandler(gormDB)
	seriesH := handlers.NewSeriesHandler(gormDB)
	aiH := handlers.NewAIHandler(mmxClient, gormDB)

	// Sub-Spec D M1 — Task 3 + Task 5 fix: /api/ai/topics accepts
	// EITHER a session cookie OR an X-API-Key. The original chain
	// (RequireAuth → MaybeAgentKey) had RequireAuth 401-ing the
	// agent caller before MaybeAgentKey could stamp agent_id —
	// caught by api-smoke.sh on 2026-06-15. RequireEitherAuth
	// composes the two checks in a single middleware: a valid
	// cookie or a valid key authorizes, neither → 401. The
	// handler's existing c.GetUint("user_id") ||
	// c.Get("agent_id") != nil gate still applies as a
	// belt-and-braces last check.
	r.POST("/api/ai/topics",
		middleware.RequireEitherAuth(gormDB, slog.Default()),
		aiH.GenerateTopics,
	)
	qualityH := handlers.NewQualityHandler(mmxClient, gormDB)
	deconstructH := handlers.NewDeconstructHandler(mmxClient)
	pipelineH := handlers.NewPipelineHandler(mmxClient, gormDB)

	// IP template routes — derived view over the knowledge_docs table.
	// Mounted after the AI handler so the URL space is owned by each
	// handler; there are no overlapping paths with the other
	// collections.
	ipTemplateH := handlers.NewIPTemplateHandler(gormDB)

	// Phase 3 observability surface. Reads aggregated call_log
	// rows; protected by RequireAuth so cross-tenant data is
	// never returned. The call_log middleware is mounted
	// INSIDE this group so a request to /api/observability/* does
	// not log itself (recursive aggregation would inflate the
	// counts).
	observabilityH := handlers.NewObservabilityHandler(gormDB)

	// Phase 3 call-log query surface. The /api/logs group is
	// multi-tenant scoped (every WHERE clause pins user_id to
	// the caller) and is added to the call_log middleware's
	// skip-list so reads of the log table do not log
	// themselves (a recursive read would inflate the by_skill
	// call count).
	logsH := handlers.NewLogsHandler(gormDB, slog.Default())

	// JSON-based data import / export endpoints. Mounted last because
	// they live at the top-level /export and /import paths and could
	// not conflict with the per-entity CRUD routes. The handler is
	// stateless beyond the *gorm.DB, so it is safe to construct after
	// every other handler has registered.
	importExportH := handlers.NewImportExportHandler(gormDB, nil)

	// Phase 2 auth handler. Lives on a PUBLIC group — it owns the
	// /api/auth/* namespace and is the only surface reachable without
	// a session cookie.
	authH := handlers.NewAuthHandler(gormDB, slog.Default())
	authH.RegisterRoutes(r)

	// Phase 2 AI batch + cover generation. The batch handler
	// reuses the same *agents.MiniMax as /ai/pipeline, and the
	// cover handler calls MiniMax.Image (with a deterministic
	// SVG fallback when no API key is configured). Both live
	// on the protected /api group so the response can be
	// scoped to the caller's rows in a future iteration (the
	// cover handler is also where the topic_id persistence
	// seam will land).
	batchH := handlers.NewBatchHandler(pipelineH.Text(), gormDB)
	coverH := handlers.NewCoverHandler(mmxClient)

	// Phase 4 media surface. SpeechHandler and VideoHandler
	// reuse the same *agents.MiniMax as the AI/cover
	// handlers — MiniMax is a single-provider client that
	// exposes Text/Image/Speech/Video on the same instance.
	// Both endpoints live on the protected /api group so the
	// call_log middleware (mounted further below) picks up
	// the row and the operator can audit cost in the
	// observability dashboard.
	speechH := handlers.NewSpeechHandler(mmxClient)
	videoH := handlers.NewVideoHandler(mmxClient)

	// Phase 4 P1 billing surface (Phase 3 console /billing
	// route). Reads aggregated call_log rows; the
	// month-summary + CSV export are operator-facing pages
	// on the console. The call_log middleware's skip-list
	// excludes /api/billing/* so a billing query does not
	// log itself.
	billingH := handlers.NewBillingHandler(gormDB)

	// Protected business surface. Every route in this group runs
	// through RequireAuth, which resolves the session cookie via
	// auth.ValidateSession and sets the user_id on the Gin context.
	// Per-handler user_id filtering is layered on top: the handler
	// stores the caller and the WHERE clauses scope reads/writes to
	// the caller's rows. Mounted under /api so the reverse proxy in
	// nginx.conf can keep /api and the static frontend in separate
	// paths.
	//
	// The call_log middleware is added to the chain so every
	// protected /api/* request gets one row. It runs AFTER
	// RequireAuth so the user_id is available; it runs BEFORE
	// the per-entity handlers so handlers can stamp provider /
	// cost / error_message on the CallLogRef.
	apiGroup := r.Group("/api",
		handlers.NewRequireAuth(gormDB, slog.Default()),
		middleware.CallLog(middleware.CallLogConfig{DB: gormDB, Logger: slog.Default()}),
	)

	// Sub-Spec C M1 — Task 2: external creator admin endpoints
	// (operator-only). Sits on a sibling /api/admin group so the
	// role-check middleware is scoped to operator-only URLs and
	// does not run on the broader /api/* CRUD surface. RequireAuth
	// must run before RequireOperatorRole (which reads user_role
	// from the context RequireAuth stamps).
	adminGroup := r.Group("/api/admin",
		handlers.NewRequireAuth(gormDB, slog.Default()),
		middleware.RequireOperatorRole(),
	)
	creatorH := handlers.NewCreatorHandler(gormDB)
	creatorH.RegisterRoutes(adminGroup)

	// Sub-Spec D M1 — Task 2: agent API key admin endpoints
	// (operator-only). Sits on the SAME /api/admin group as the
	// creator endpoints above — both surfaces share the
	// RequireAuth + RequireOperatorRole chain. IMPORTANT: pass
	// the router group (not the absolute path) so routes
	// register with RELATIVE paths. The Sub-Spec C Task 2
	// 'RegisterRoutes absolute path' bug doubled the prefix and
	// 4xx'd every admin endpoint until smoke caught it in
	// Task 5. We use relative paths here from the start.
	agentH := handlers.NewAgentHandler(gormDB)
	agentH.RegisterRoutes(adminGroup)

	topicH.RegisterRoutes(apiGroup)
	scriptH.RegisterRoutes(apiGroup)
	contentItemH.RegisterRoutes(apiGroup)
	knowledgeSearchH.RegisterRoutes(apiGroup)
	knowledgeDocH.RegisterRoutes(apiGroup)
	seriesH.RegisterRoutes(apiGroup)
	// aiH is registered partly above (/api/ai/topics with the
	// RequireAuth + MaybeAgentKey chain) and partly here
	// (/api/ai/humanize + /api/ai/postmortem on the standard
	// apiGroup). We do NOT call aiH.RegisterRoutes(apiGroup)
	// because that would double-register /api/ai/topics and gin
	// would panic at startup. The two /ai/* endpoints below stay
	// session-cookie-only for now; Task 4+ can opt them into the
	// same chain if/when agent callers need them.
	apiGroup.POST("/ai/humanize", aiH.HumanizeScript)
	apiGroup.POST("/ai/postmortem", aiH.Postmortem)
	qualityH.RegisterRoutes(apiGroup)
	deconstructH.RegisterRoutes(apiGroup)
	pipelineH.RegisterRoutes(apiGroup)
	ipTemplateH.RegisterRoutes(apiGroup)
	importExportH.RegisterRoutes(apiGroup)
	batchH.RegisterRoutes(apiGroup)
	coverH.RegisterRoutes(apiGroup)
	speechH.RegisterRoutes(apiGroup)
	videoH.RegisterRoutes(apiGroup)
	credentialH.RegisterRoutes(apiGroup)
	observabilityH.RegisterRoutes(apiGroup)
	logsH.RegisterRoutes(apiGroup)
	billingH.RegisterRoutes(apiGroup)

	// API reference surface — /openapi.json serves the OpenAPI 3.0
	// spec embedded at compile time, /docs serves the Swagger UI
	// shell. Mounted after every other handler so they cannot be
	// shadowed by a future entity handler claiming the top-level
	// /openapi.json or /docs path.
	docsH := handlers.NewDocsHandlers(slog.Default())
	docsH.RegisterRoutes(r)

	// /static serves the on-disk media outputs (cover jpgs +
	// speech mp3s + future video mp4s). Mounted OUTSIDE the /api
	// group so the URLs match the relative path under ./var/
	// (e.g. var/covers/foo.jpg → /static/covers/foo.jpg). The
	// files are not authenticated — they are inert binary
	// content with no per-user access control today, and
	// gating them would force the <img>/<audio> tags in the
	// frontend to carry a session cookie (which is fine, but
	// adds complexity for zero security benefit when the URLs
	// are not enumerable). A future iteration can move this
	// behind a signed-URL helper if cross-tenant leak becomes
	// a concern.
	r.Static("/static", "./var")

	httpSrv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		// WriteTimeout raised to 180s to accommodate the 4-step AI pipeline at
		// POST /api/ai/pipeline, which reliably runs 1m28s-1m38s end-to-end
		// (4 sequential LLM calls). 60s cut the response mid-body.
		WriteTimeout: 180 * time.Second,
		IdleTimeout:  120 * time.Second,
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
	// HTTP side finish first. We wait for the MCP goroutine to
	// return (bounded by a timeout) instead of sleeping — the
	// WaitGroup completes when ServeStdio actually exits after
	// mcpCancel.
	mcpCancel()
	mcpDone := make(chan struct{})
	go func() {
		mcpWG.Wait()
		close(mcpDone)
	}()
	select {
	case <-mcpDone:
	case <-time.After(2 * time.Second):
		slog.Warn("mcp shutdown timed out")
	}

	slog.Info("shutdown complete")
}

// setupLogger configures the process-wide slog default. The handler
// is picked once based on the LOG_FORMAT env var — the previous
// implementation set the JSON handler unconditionally and then
// re-set a text handler when LOG_FORMAT was unset, leaving the JSON
// setup as dead code.
func setupLogger(format string) {
	var handler slog.Handler
	switch format {
	case "json":
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})
	default:
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})
	}
	slog.SetDefault(slog.New(handler))
}

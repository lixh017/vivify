package handlers

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/opc/api/internal/docs"
)

// swaggerUIHTML is the static HTML page served at /docs. It bootstraps
// Swagger UI from the jsDelivr CDN and points it at /openapi.json on the
// same origin. Inlining the HTML keeps the docs surface dependency-free:
// no template engine, no static-file middleware, no extra CSP entries.
//
// We pin a specific swagger-ui-dist version (5.17.14) so a future CDN
// release cannot silently swap the bundle under us. SRI hashes are
// omitted intentionally: jsDelivr rotates file paths across releases
// and we want the docs page to keep working even when the operator
// forgets to refresh a hard-coded hash after a swagger-ui upgrade.
const swaggerUIHTML = `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>OPC API - Swagger UI</title>
    <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5.17.14/swagger-ui.css" crossorigin="anonymous" />
    <style>
      html, body { margin: 0; padding: 0; height: 100%; }
      #swagger-ui { height: 100vh; }
    </style>
  </head>
  <body>
    <div id="swagger-ui"></div>
    <script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5.17.14/swagger-ui-bundle.js" crossorigin="anonymous"></script>
    <script>
      window.onload = () => {
        window.ui = SwaggerUIBundle({
          url: "/openapi.json",
          dom_id: "#swagger-ui",
          deepLinking: true,
          presets: [SwaggerUIBundle.presets.apis],
        });
      };
    </script>
  </body>
</html>
`

// DocsHandlers bundles the HTTP handlers that serve the API reference:
// the OpenAPI 3.0 spec at /openapi.json and the Swagger UI shell at /docs.
//
// The spec bytes are baked in at compile time via //go:embed in the
// internal/docs package, so the docs surface is always in lock-step
// with the binary that serves it (no risk of a stale spec being
// served from disk after a rollback).
type DocsHandlers struct {
	logger *slog.Logger
}

// NewDocsHandlers returns a DocsHandlers wired to slog.Default() when
// no logger is provided. The handlers are stateless, so the zero value
// is safe to share across goroutines.
func NewDocsHandlers(logger *slog.Logger) *DocsHandlers {
	if logger == nil {
		logger = slog.Default()
	}
	return &DocsHandlers{logger: logger}
}

// RegisterRoutes attaches /openapi.json and /docs to the given router.
// We register /openapi.json with its full content type so the Swagger
// UI client can fetch the spec directly without sniffing.
func (h *DocsHandlers) RegisterRoutes(r gin.IRouter) {
	r.GET("/openapi.json", h.OpenAPISpec)
	r.GET("/docs", h.SwaggerUI)
	// Trailing-slash alias so /docs/ also works — convenient when the
	// page links to itself relatively. Gin would otherwise 404.
	r.GET("/docs/", h.SwaggerUI)
}

// OpenAPISpec — GET /openapi.json
//
// Serves the embedded OpenAPI 3.0 document. We pass through the raw
// bytes rather than re-marshaling so the on-the-wire bytes match the
// file on disk byte-for-byte (no key reordering, no whitespace
// normalization). The Cache-Control header tells the browser to
// revalidate every time — a docs spec is small and the user almost
// always wants the latest version after a server restart.
func (h *DocsHandlers) OpenAPISpec(c *gin.Context) {
	c.Header("Cache-Control", "no-cache, must-revalidate")
	c.Data(http.StatusOK, "application/json; charset=utf-8", docs.OpenAPIJSON())
}

// SwaggerUI — GET /docs
//
// Serves the static HTML that bootstraps Swagger UI from the CDN.
// No request body, no parameters. The HTML asks the browser to fetch
// /openapi.json on the same origin, so the spec and the UI are always
// served by the same binary.
func (h *DocsHandlers) SwaggerUI(c *gin.Context) {
	c.Header("Cache-Control", "no-cache, must-revalidate")
	c.Header("Content-Type", "text/html; charset=utf-8")
	if _, err := c.Writer.WriteString(swaggerUIHTML); err != nil {
		// We've already written headers, so the only thing left is to
		// log and abort. The client will see a truncated body.
		h.logger.Warn("docs: write swagger UI html failed", "err", err.Error(), "request_id", c.GetString("request_id"))
		_ = c.Error(err) //nolint:errcheck
		c.Abort()
	}
}

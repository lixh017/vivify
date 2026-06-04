// Package middleware contains Gin middlewares shared across the API.
//
// The request logger writes one structured slog record per HTTP request once
// the handler chain returns. It deliberately uses the package-level slog
// default logger so callers can route the logs (JSON for production, text
// for local dev) via slog.SetDefault without touching the middleware.
package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

// RequestLogger logs method, path, status, latency and the request id (set
// by handlers.RequestID) for every completed request. Failures and
// non-2xx statuses are logged at WARN; everything else at INFO. The
// request id is read from the gin context — it is not required, and the
// log key is simply omitted when no id was assigned upstream.
func RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()

		// Build the attribute slice as []slog.Attr directly so the
		// type checker validates every entry at the call site and
		// slog.LogAttrs avoids a []any round-trip.
		attrs := []slog.Attr{
			slog.String("method", c.Request.Method),
			slog.String("path", c.Request.URL.Path),
			slog.Int("status", status),
			slog.Duration("latency", latency),
			slog.String("client_ip", c.ClientIP()),
		}
		if v, ok := c.Get("request_id"); ok {
			if id, ok := v.(string); ok && id != "" {
				attrs = append(attrs, slog.String("request_id", id))
			}
		}

		level := slog.LevelInfo
		if status >= 500 {
			level = slog.LevelError
		} else if status >= 400 {
			level = slog.LevelWarn
		}
		slog.LogAttrs(c.Request.Context(), level, "http request", attrs...)
	}
}

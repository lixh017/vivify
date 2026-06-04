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

		attrs := []any{
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
		slog.LogAttrs(c.Request.Context(), level, "http request", asAttrs(attrs)...)
	}
}

// asAttrs converts a []any of slog.Attr into the typed slice slog expects.
// Anything that is not a slog.Attr is dropped — we never construct
// heterogeneous values at the call site, but the helper keeps the type
// checker happy if the slice contents change.
func asAttrs(in []any) []slog.Attr {
	out := make([]slog.Attr, 0, len(in))
	for _, v := range in {
		if a, ok := v.(slog.Attr); ok {
			out = append(out, a)
		}
	}
	return out
}

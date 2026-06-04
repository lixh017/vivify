// Package middleware: cors.go configures CORS for the API.
//
// Phase 2 ships session-based auth backed by an HttpOnly cookie. The
// browser will only send the cookie on cross-origin XHR/Fetch if the
// server opts in with `Access-Control-Allow-Credentials: true` AND
// echoes a concrete origin in `Access-Control-Allow-Origin` (the
// spec forbids the wildcard `*` in that case). This file is the
// single place that policy is expressed; handlers should not
// manipulate the CORS headers themselves.
package middleware

import (
	"os"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// allowedOriginEnv names the env var operators set to the public
// origin of their Next.js frontend (e.g. https://opc.example.com). The
// fallback is the loopback dev origin so `go run ./cmd/server` works
// out of the box against `pnpm dev` on the same machine.
const allowedOriginEnv = "NEXT_PUBLIC_API_URL"

// devOrigin is the fallback allowed origin. It is intentionally a
// concrete value (not "*") so the Allow-Origins list stays compatible
// with Allow-Credentials — browsers reject the wildcard when the
// server opts into cookie auth.
const devOrigin = "http://localhost:3000"

// resolveAllowedOrigins returns the list of origins the CORS
// middleware should echo back. We always include the dev origin so
// local development works without env-var setup, and we add whatever
// the operator set via NEXT_PUBLIC_API_URL.
//
// A comma-separated list is accepted (some deployments front the API
// with multiple domain names — staging, prod, a preview URL) and
// each entry is trimmed of whitespace.
func resolveAllowedOrigins() []string {
	origins := []string{devOrigin}
	raw := strings.TrimSpace(os.Getenv(allowedOriginEnv))
	if raw == "" {
		return origins
	}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		origins = append(origins, part)
	}
	return origins
}

// CORS returns a Gin middleware configured for the Phase 2 cookie
// auth surface:
//
//   - AllowOrigins: NEXT_PUBLIC_API_URL or the loopback dev origin.
//     Must be a concrete origin, not "*", because we set
//     AllowCredentials to true (browsers reject the combination).
//   - AllowCredentials: true — the browser will send the opc_session
//     cookie on cross-origin XHR/Fetch from the configured frontend.
//   - AllowMethods: the methods the API actually uses, plus OPTIONS
//     for preflight. Restricting the list (vs AllowAllOrigins) keeps
//     the surface area small and makes accidental CSRF by an
//     unexpected verb impossible.
//   - AllowHeaders: Content-Type for JSON bodies, Authorization for
//     API clients that prefer Bearer tokens over cookies, and
//     X-Request-ID so the frontend can correlate logs end-to-end.
//   - ExposeHeaders: X-Request-ID — we want the client to be able to
//     read it back so support tickets can quote a single trace id.
//   - MaxAge: 12 hours — long enough to keep preflight cheap during
//     a dev session, short enough that a misconfigured frontend is
//     noticed within a working day.
func CORS() gin.HandlerFunc {
	return cors.New(cors.Config{
		AllowOrigins:     resolveAllowedOrigins(),
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "X-Request-ID"},
		ExposeHeaders:    []string{"X-Request-ID"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	})
}

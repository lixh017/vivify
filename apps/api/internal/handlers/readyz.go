package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// Pinger is the minimal capability /readyz needs from a database
// handle. The production code passes the *sql.DB hidden behind
// *gorm.DB; tests can supply a fake without depending on a real
// database. We keep it small (one method) so adapters are trivial.
type Pinger interface {
	PingContext(ctx context.Context) error
}

// readyzTimeout bounds the time the readiness probe is willing to
// wait for the database. Short by design — a kubelet probe that
// blocks the goroutine for too long defeats the purpose of the
// timeout.
const readyzTimeout = 2 * time.Second

// NewReadyz returns a /readyz handler that reports the process's
// ability to serve traffic. It pings the supplied Pinger (typically
// the project's *sql.DB) with a short timeout and returns 200 when
// the ping succeeds, 503 otherwise.
//
// The detailed error is logged server-side so operators can
// diagnose the failure; the response body only carries a generic
// "reason" string so a probe cannot be used to fingerprint driver
// versions, file paths, or other low-level internals.
func NewReadyz(pinger Pinger) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), readyzTimeout)
		defer cancel()

		if err := pinger.PingContext(ctx); err != nil {
			slog.WarnContext(c.Request.Context(), "readyz: db ping failed", "error", err)
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "unready",
				"reason": "db ping failed",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	}
}

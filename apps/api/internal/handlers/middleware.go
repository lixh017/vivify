package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/opc/api/internal/middleware"
	"github.com/opc/api/internal/models"
)

// RequestID assigns or propagates an X-Request-ID header and stashes the
// value in the context for downstream log lines. Cheap, allocation-free on
// the hot path except for the random hex.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-ID")
		if id == "" {
			var b [8]byte
			_, _ = rand.Read(b[:])
			id = hex.EncodeToString(b[:])
		}
		c.Set("request_id", id)
		c.Writer.Header().Set("X-Request-ID", id)
		c.Next()
	}
}

// NewRequireAuth returns the session-cookie auth middleware. The
// canonical implementation lives in internal/middleware (so the
// handlers package's *_test.go files can import middleware for
// Metrics() without an import cycle). This wrapper exists so the
// call sites in main.go and the auth handler tests continue to
// import "internal/handlers" without churn.
func NewRequireAuth(db *gorm.DB, logger *slog.Logger) gin.HandlerFunc {
	return middleware.RequireAuth(db, logger)
}

// UserIDFromContext returns the authenticated user ID stored by
// RequireAuth. Thin re-export so the existing per-entity handlers
// (which all call handlers.UserIDFromContext) do not need a second
// import.
func UserIDFromContext(c *gin.Context) uint {
	return middleware.UserIDFromContext(c)
}

// UserFromContext returns the full user stored by RequireAuth. Thin
// re-export — see UserIDFromContext.
func UserFromContext(c *gin.Context) (models.User, bool) {
	return middleware.UserFromContext(c)
}

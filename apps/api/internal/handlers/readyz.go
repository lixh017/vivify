package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// NewReadyz returns a /readyz handler that reports the process's ability
// to serve traffic. The current implementation pings the database with a
// short timeout — a healthy dependency graph that includes a working DB
// is the contract we want kubelet / load balancer probes to enforce.
//
// Returns 200 when the DB responds to a ping within the timeout, 503
// otherwise. The body always carries a JSON status payload so probes
// that surface it (e.g. a human running curl) can tell *why* the
// readiness check failed.
func NewReadyz(gormDB *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()

		sqlDB, err := gormDB.DB()
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "unready",
				"reason": "db handle unavailable",
				"error":  err.Error(),
			})
			return
		}
		if err := sqlDB.PingContext(ctx); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "unready",
				"reason": "db ping failed",
				"error":  err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	}
}

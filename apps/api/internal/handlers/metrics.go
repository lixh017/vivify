package handlers

import (
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// MetricsHandler adapts promhttp.Handler for gin. The standard
// promhttp handler expects an http.Handler; gin.WrapH converts it
// into a gin.HandlerFunc. We pass nil to indicate that the default
// gatherer (which includes the package-level metrics registered in
// internal/middleware) should be used — this keeps registration
// implicit and avoids the need to plumb a *prometheus.Registry
// through the handler.
func MetricsHandler() gin.HandlerFunc {
	h := promhttp.Handler()
	return gin.WrapH(h)
}

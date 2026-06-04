package handlers

import (
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// MetricsHandler adapts promhttp.Handler for gin. The standard
// promhttp handler expects an http.Handler; gin.WrapH converts it
// into a gin.HandlerFunc.
//
// By default, promhttp.Handler() gathers from the package-level
// default registry, which is where the counters and histograms
// defined in internal/middleware are auto-registered (via
// promauto). Passing a non-nil reg scopes the handler to that
// registry, which is the right shape for tests that want a
// hermetic exposition without contamination from process-global
// state. Production wiring (cmd/server) calls MetricsHandler()
// with no arguments and gets the default-gatherer behavior.
//
// IMPORTANT: if a custom registry is supplied, callers must also
// register their metrics on that same registry — the middleware's
// package-level promauto metrics are NOT visible to a custom
// gatherer.
func MetricsHandler(reg ...*prometheus.Registry) gin.HandlerFunc {
	var h gin.HandlerFunc
	if len(reg) > 0 && reg[0] != nil {
		h = gin.WrapH(promhttp.HandlerFor(reg[0], promhttp.HandlerOpts{}))
	} else {
		h = gin.WrapH(promhttp.Handler())
	}
	return h
}

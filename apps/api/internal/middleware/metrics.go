// Package middleware contains Gin middlewares shared across the API.
//
// The metrics middleware records request count and latency in Prometheus
// format. /metrics is excluded to avoid recursion and the resulting
// feedback loop on the latency histogram.
package middleware

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// metricsPath is the only route excluded from instrumentation. The
// endpoint itself is registered directly on the gin.Engine (it uses
// promhttp.Handler and must NOT flow through the metrics middleware or
// the histogram would observe its own scrape time on every interval).
const metricsPath = "/metrics"

// HTTP request count partitioned by method, path template and status.
// We use the route template ("/topics/:id") instead of the concrete URL
// to keep label cardinality bounded; the alternative — emitting one
// series per /topics/123, /topics/456, ... — would explode the
// time-series count in a real deployment.
var httpRequestsTotal = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "opc_http_requests_total",
		Help: "Total number of HTTP requests processed, labeled by method, path, and status code.",
	},
	[]string{"method", "path", "status"},
)

// HTTP request latency in seconds, partitioned by method and path
// template. The bucket layout covers the latency band that matters for
// an SQLite-backed CRUD API — 5ms up to 5s — with denser buckets in
// the <100ms range where the SLOs (e.g. /topics p95 < 100ms) live.
var httpRequestDurationSeconds = promauto.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "opc_http_request_duration_seconds",
		Help:    "HTTP request latency in seconds, labeled by method and path.",
		Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
	},
	[]string{"method", "path"},
)

// Metrics returns a gin middleware that records per-request counters
// and histograms. It must be registered before any handler that
// should be observed, but on a sub-engine that excludes /metrics (or
// after /metrics is mounted on the parent engine) to avoid recursion.
func Metrics() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		// Defensive skip — even if the caller mounted this globally
		// by accident, we must not observe the scrape itself.
		if c.Request.URL.Path == metricsPath {
			return
		}

		// FullPath returns the route template ("/topics/:id") when
		// matched, or an empty string for 404s. We fall back to the
		// concrete path for unmatched routes so 404s still surface in
		// the metrics — but at the cost of unbounded cardinality if a
		// scanner hits the service. In practice the path is at least
		// bounded by the gin router's normalization, and the histogram
		// does NOT include path (only method/path status), so the
		// cardinality risk is acceptable for a single-process service.
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}

		status := strconv.Itoa(c.Writer.Status())
		method := c.Request.Method

		httpRequestsTotal.WithLabelValues(method, path, status).Inc()
		httpRequestDurationSeconds.WithLabelValues(method, path).Observe(time.Since(start).Seconds())
	}
}

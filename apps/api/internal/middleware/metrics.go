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

// HTTP request latency in seconds, partitioned by method, matched
// route template, and status code. The label set mirrors
// httpRequestsTotal so SLO dashboards can slice p95 by status
// (e.g. p95 of 5xx vs 2xx) without joining the two metric families.
// The bucket layout covers the latency band that matters for an
// SQLite-backed CRUD API — 5ms up to 5s — with denser buckets in
// the <100ms range where the SLOs (e.g. /topics p95 < 100ms) live.
//
// Cardinality note: the histogram is intentionally restricted to
// MATCHED routes (c.FullPath() != ""). Unmatched routes (404s) are
// only counted via httpRequestsTotal, where the URL.Path fallback
// exists with a documented cardinality trade-off (see below). The
// histogram carries ~9 buckets per series, so a leaked path label
// would be far more expensive on the histogram than on the counter.
var httpRequestDurationSeconds = promauto.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "opc_http_request_duration_seconds",
		Help:    "HTTP request latency in seconds, labeled by method, path, and status code.",
		Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
	},
	[]string{"method", "path", "status"},
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
		// the counter — at the cost of unbounded cardinality if a
		// scanner hits the service. This trade-off is acceptable for
		// a single-process service because:
		//   1) The histogram is gated on a matched route, so a leaked
		//      path label only inflates the counter (cheap) and not
		//      the histogram (expensive: ~10 buckets per series).
		//   2) Operators who care can drop the 404 fallback in a
		//      follow-up; the bounded route-template label is the
		//      default in real usage.
		path := c.FullPath()
		method := c.Request.Method
		status := strconv.Itoa(c.Writer.Status())

		// Counter records everything (matched + 404 fallback).
		if path == "" {
			path = c.Request.URL.Path
		}
		httpRequestsTotal.WithLabelValues(method, path, status).Inc()

		// Histogram is restricted to matched routes only. For 404s
		// we record nothing on the histogram — the counter series
		// with the URL.Path label is the operator's signal that a
		// scan is in progress.
		if route := c.FullPath(); route != "" {
			httpRequestDurationSeconds.WithLabelValues(method, route, status).Observe(time.Since(start).Seconds())
		}
	}
}

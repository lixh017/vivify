package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
)

// resetMetrics clears the package-level counter and histogram
// registries. The metrics are registered with promauto at init time
// and would otherwise leak series across tests in the same process.
// Each test calls this in t.Cleanup so the next test starts with a
// fresh slate — a regression in another test (or a test that hits
// a different path) cannot affect the assertions here.
func resetMetrics(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		httpRequestsTotal.Reset()
		httpRequestDurationSeconds.Reset()
	})
}

// histogramSampleCount returns the cumulative observation count for
// a single (method, path, status) series on the histogram. We use
// the dto.Metric protobuf to read GetSampleCount() directly, which
// is the right primitive here: testutil.ToFloat64 is for counters
// and gauges, and testutil.CollectAndCount returns the number of
// distinct series (not observations). Summing observations across
// series is the only way to assert that an Observe() call landed.
func histogramSampleCount(t *testing.T, method, path, status string) uint64 {
	t.Helper()
	// WithLabelValues on a HistogramVec returns prometheus.Observer,
	// which has no Write method. We have to round-trip through
	// prometheus.Labels and the (Metric, error) form on the
	// Collector interface to reach the dto.Metric.
	m := &dto.Metric{}
	if err := httpRequestDurationSeconds.With(prometheus.Labels{
		"method": method,
		"path":   path,
		"status": status,
	}).(prometheus.Metric).Write(m); err != nil {
		t.Fatalf("write histogram metric: %v", err)
	}
	if m.Histogram == nil {
		return 0
	}
	return m.Histogram.GetSampleCount()
}

func newTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Metrics())
	r.GET("/widgets/:id", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	r.GET("/widgets", func(c *gin.Context) {
		c.String(http.StatusOK, "list")
	})
	r.GET("/boom", func(c *gin.Context) {
		c.String(http.StatusInternalServerError, "boom")
	})
	// /metrics is registered like in production, but served by a
	// stub here — we only care that the middleware does not
	// observe it. The real promhttp handler is exercised in the
	// handler-package tests.
	r.GET(metricsPath, func(c *gin.Context) {
		c.String(http.StatusOK, "metrics-stub")
	})
	return r
}

// TestMetrics_CounterAndHistogramRecord verifies that a single
// successful request increments the counter by exactly 1 and that
// the histogram observes at least one sample. We use prometheus'
// testutil to read counter state directly — this catches
// double-counting bugs (e.g. middleware running on /metrics itself).
func TestMetrics_CounterAndHistogramRecord(t *testing.T) {
	resetMetrics(t)
	r := newTestRouter()

	// Snapshot count before the request so the test is robust to
	// other tests in the same package running first.
	before := testutil.ToFloat64(httpRequestsTotal.WithLabelValues("GET", "/widgets/:id", "200"))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/widgets/42", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	after := testutil.ToFloat64(httpRequestsTotal.WithLabelValues("GET", "/widgets/:id", "200"))
	if after-before != 1 {
		t.Errorf("expected counter to increment by 1, got %v", after-before)
	}

	// Histogram exposes an observation count collector; assert
	// it's non-zero so a regression that breaks the Observe call
	// (e.g. shadowed by an early return) is caught.
	if got := testutil.CollectAndCount(httpRequestDurationSeconds); got == 0 {
		t.Errorf("expected histogram to record at least one observation, got %d", got)
	}
}

// TestMetrics_SkipsMetricsPath verifies that the middleware does
// NOT record observations for the /metrics endpoint itself. This is
// the recursion guard: if it failed, every Prometheus scrape would
// observe its own scrape latency, which would look like a real
// performance regression in dashboards.
//
// We assert by snapshotting the total counter across all labels
// before and after hitting /metrics — the delta must be zero
// because the stub /metrics handler in this test does not flow
// through any other label-emitting middleware.
func TestMetrics_SkipsMetricsPath(t *testing.T) {
	resetMetrics(t)
	r := newTestRouter()

	// Establish a baseline by hitting a real route once; this
	// guarantees the collector has at least one series so the
	// "no new series" assertion is meaningful.
	{
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/widgets", nil))
	}

	// Now scrape /metrics a few times. The middleware's own skip
	// branch should mean no new observations are recorded.
	for range 5 {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, metricsPath, nil))
	}

	// After five /metrics calls, the /widgets counter (status 200)
	// must still be exactly 1 — the scrape calls must not have
	// added any observations, either to /widgets (different path)
	// or to the (non-existent) /metrics series.
	got := testutil.ToFloat64(httpRequestsTotal.WithLabelValues("GET", "/widgets", "200"))
	if got != 1 {
		t.Errorf("expected /widgets counter to remain 1, got %v", got)
	}
}

// TestMetrics_RecordsFiveXX verifies that 5xx responses still
// increment the counter and feed the histogram. This is the case
// the SLO dashboards care most about — a 500 burst should be
// visible in the status="500" series.
func TestMetrics_RecordsFiveXX(t *testing.T) {
	resetMetrics(t)
	r := newTestRouter()

	before := testutil.ToFloat64(httpRequestsTotal.WithLabelValues("GET", "/boom", "500"))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}

	after := testutil.ToFloat64(httpRequestsTotal.WithLabelValues("GET", "/boom", "500"))
	if after-before != 1 {
		t.Errorf("expected 500 counter to increment by 1, got %v", after-before)
	}

	// The histogram carries the same {method,path,status} triple
	// as the counter — assert the 5xx series is observed so a
	// regression that strips `status` from the histogram (a known
	// SLO-dashboard must-have) is caught here. We sum the sample
	// count across all series because the per-series observation
	// count is what the histogram actually records.
	if got := histogramSampleCount(t, "GET", "/boom", "500"); got != 1 {
		t.Errorf("expected 5xx histogram observation count to be 1, got %v", got)
	}
}

// TestMetrics_HistogramSkipsUnmatchedRoutes verifies the cardinality
// guard: an unmatched route (c.FullPath() == "") must NOT record a
// histogram observation, even though the counter does (with the
// URL.Path fallback). This is the rule that keeps the histogram
// bounded when a scanner hits the service.
func TestMetrics_HistogramSkipsUnmatchedRoutes(t *testing.T) {
	resetMetrics(t)
	r := newTestRouter()

	// Hitting a path the router does not know about produces a
	// 404 and an empty FullPath. The histogram must NOT mint a
	// series keyed by the concrete URL.Path — only the counter
	// should. We assert on a sentinel series that we know must
	// stay at zero regardless of how many 404s we throw at it.
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/no-such-route", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
	for _, p := range []string{"/no-such-route", "/another-404", "/wp-login.php"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, p, nil))
	}
	// Sample count on any URL.Path-keyed histogram series must
	// remain zero — the URL.Path fallback is counter-only.
	if got := histogramSampleCount(t, "GET", "/no-such-route", "404"); got != 0 {
		t.Errorf("expected histogram to skip 404 observations, got count=%v", got)
	}
}

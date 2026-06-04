package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/opc/api/internal/middleware"
)

// TestMetricsEndpoint_ServesPrometheusTextFormat verifies the
// /metrics endpoint responds with the Prometheus text exposition
// format and the well-known metric families are present. We install
// the metrics middleware in the test router so a request to /probe
// actually increments opc_http_requests_total — otherwise the
// counter would have no labelled series and the exposition would
// not contain the metric name.
func TestMetricsEndpoint_ServesPrometheusTextFormat(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.Metrics())
	// Drive at least one request through a labelled handler so
	// opc_http_requests_total has a non-zero series to expose.
	r.GET("/probe", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	r.GET("/metrics", MetricsHandler())

	probe := httptest.NewRecorder()
	r.ServeHTTP(probe, httptest.NewRequest(http.MethodGet, "/probe", nil))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 from /metrics, got %d", w.Code)
	}

	ct := w.Header().Get("Content-Type")
	// Prometheus text format starts with "text/plain" and carries
	// the version=0.0.4 charset parameter. We assert on the
	// version token — the exact charset string has shifted across
	// client_golang releases and is not part of the contract we
	// depend on.
	if !strings.Contains(ct, "version=0.0.4") {
		t.Errorf("expected Prometheus text Content-Type, got %q", ct)
	}

	body := w.Body.String()
	// Both the counter and the histogram registered in
	// internal/middleware must appear in the exposition.
	if !strings.Contains(body, "opc_http_requests_total") {
		t.Errorf("expected opc_http_requests_total in body, got:\n%s", body)
	}
	if !strings.Contains(body, "opc_http_request_duration_seconds") {
		t.Errorf("expected opc_http_request_duration_seconds in body, got:\n%s", body)
	}
}

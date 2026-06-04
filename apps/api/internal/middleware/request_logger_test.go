package middleware

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// captureLogger swaps the package default slog handler for one that
// writes to a buffer, runs fn, then restores the previous default. The
// return value is the captured log output.
func captureLogger(t *testing.T, fn func()) string {
	t.Helper()
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })

	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))
	fn()
	return buf.String()
}

func TestRequestLoggerEmitsStructuredFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("request_id", "test-req-id-123")
		c.Next()
	})
	r.Use(RequestLogger())
	r.GET("/ok", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	output := captureLogger(t, func() {
		req, _ := http.NewRequest("GET", "/ok", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("expected 200, got %d", w.Code)
		}
	})

	// Find the request log line — the only one with "http request" msg.
	lines := strings.Split(strings.TrimSpace(output), "\n")
	var record map[string]any
	for _, line := range lines {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("non-JSON log line %q: %v", line, err)
		}
		if m["msg"] == "http request" {
			record = m
			break
		}
	}
	if record == nil {
		t.Fatalf("no request log line found; output=%s", output)
	}

	wantFields := map[string]any{
		"method":     "GET",
		"path":       "/ok",
		"status":     float64(200),
		"request_id": "test-req-id-123",
	}
	for k, v := range wantFields {
		if got := record[k]; got != v {
			t.Errorf("field %q: got %v, want %v", k, got, v)
		}
	}
	if _, ok := record["latency"]; !ok {
		t.Errorf("expected latency field in log record")
	}
}

func TestRequestLoggerWarnsOn4xxAndErrorsOn5xx(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestLogger())
	r.GET("/bad", func(c *gin.Context) { c.JSON(400, gin.H{}) })
	r.GET("/oops", func(c *gin.Context) { c.JSON(500, gin.H{}) })

	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	for _, path := range []string{"/bad", "/oops"} {
		req, _ := http.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	levels := map[string]string{}
	for _, line := range lines {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("non-JSON line %q", line)
		}
		if m["msg"] == "http request" {
			levels[m["path"].(string)] = m["level"].(string)
		}
	}
	if got := levels["/bad"]; got != "WARN" {
		t.Errorf("/bad expected WARN level, got %s", got)
	}
	if got := levels["/oops"]; got != "ERROR" {
		t.Errorf("/oops expected ERROR level, got %s", got)
	}
}

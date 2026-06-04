package handlers

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newTestGorm wires up a shared-cache in-memory SQLite GORM handle.
// The shared cache lets PingContext reach a connection that has
// already been initialized; an unnamed ":memory:" DSN gives a fresh
// database per connection in some sqlite drivers, which would break
// Ping.
func newTestGorm(t *testing.T) *gorm.DB {
	t.Helper()
	gormDB, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	return gormDB
}

// pingerFromGorm resolves a *gorm.DB to its underlying *sql.DB so
// the test can drive NewReadyz through the same interface the
// production main.go uses.
func pingerFromGorm(t *testing.T, gormDB *gorm.DB) *sql.DB {
	t.Helper()
	sqlDB, err := gormDB.DB()
	if err != nil {
		t.Fatalf("get sql.DB: %v", err)
	}
	return sqlDB
}

func TestReadyzReturns200OnHealthyDB(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/readyz", NewReadyz(pingerFromGorm(t, newTestGorm(t))))

	req, _ := http.NewRequest("GET", "/readyz", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d, body=%s", w.Code, w.Body.String())
	}
}

func TestReadyzReturns503OnClosedDB(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sqlDB := pingerFromGorm(t, newTestGorm(t))
	// Close the underlying *sql.DB so the next ping fails. We do not
	// need the *gorm.DB handle for this test.
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close sql.DB: %v", err)
	}

	r := gin.New()
	r.GET("/readyz", NewReadyz(sqlDB))

	req, _ := http.NewRequest("GET", "/readyz", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d, body=%s", w.Code, w.Body.String())
	}
	// The response must not echo the raw driver error — that would
	// leak internals to anyone probing /readyz.
	body := w.Body.String()
	if want := `"reason":"db ping failed"`; !strings.Contains(body, want) {
		t.Errorf("expected reason %q in body, got %s", want, body)
	}
	if strings.Contains(body, "sql:") || strings.Contains(body, "database is closed") {
		t.Errorf("body leaked driver error: %s", body)
	}
}

// fakePinger is a small Pinger that always returns the supplied
// error. Used to prove NewReadyz's failure path without depending on
// a real database.
type fakePinger struct{ err error }

func (f *fakePinger) PingContext(_ context.Context) error { return f.err }

func TestReadyzUsesPingerInterface(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/readyz", NewReadyz(&fakePinger{err: errors.New("synthetic")}))

	req, _ := http.NewRequest("GET", "/readyz", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d, body=%s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "synthetic") {
		t.Errorf("response leaked the synthetic error: %s", w.Body.String())
	}
}

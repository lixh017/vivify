package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newTestGorm(t *testing.T) *gorm.DB {
	t.Helper()
	// Use a shared in-memory DSN with a name so the connection survives
	// the PingContext call (unnamed ":memory:" gives a fresh DB per
	// connection in some sqlite drivers, which would break Ping).
	gormDB, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	return gormDB
}

func TestReadyzReturns200OnHealthyDB(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/readyz", NewReadyz(newTestGorm(t)))

	req, _ := http.NewRequest("GET", "/readyz", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d, body=%s", w.Code, w.Body.String())
	}
}

func TestReadyzReturns503OnClosedDB(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := newTestGorm(t)
	// Close the underlying sql.DB so the next ping fails.
	sqlDB, err := gormDB.DB()
	if err != nil {
		t.Fatalf("get sql.DB: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close sql.DB: %v", err)
	}

	r := gin.New()
	r.GET("/readyz", NewReadyz(gormDB))

	req, _ := http.NewRequest("GET", "/readyz", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d, body=%s", w.Code, w.Body.String())
	}
}

//go:build fts5

package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/opc/api/internal/db"
)

// ensureSearchFTS brings the FTS5 surface online via the shared
// db.EnsureKnowledgeFTS helper. Tests in this file (all build-tagged
// fts5) call it instead of re-implementing the DDL, so the schema
// lives in exactly one place.
func ensureSearchFTS(gormDB *gorm.DB) error {
	return db.EnsureKnowledgeFTS(gormDB)
}

// doBench mirrors the do() test helper but accepts *testing.B so
// benchmarks can call the handler in the inner loop. Kept here (and
// not in topic_test.go) because the bench is the only consumer.
func doBench(b *testing.B, r *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	b.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			b.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, path, reader)
	if err != nil {
		b.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

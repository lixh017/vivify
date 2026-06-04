package handlers

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/opc/api/internal/models"
)

// TestKnowledgeRouteOrdering is a regression test for the
// /knowledge/search vs /knowledge/:id route conflict documented in
// cmd/server/main.go. Gin's radix tree prefers static segments over
// :id wildcards, so /knowledge/search must reach the search handler
// (which returns 400 on empty q) rather than the CRUD Get handler
// (which returns 400 on non-numeric id). If a future refactor breaks
// the static-over-wildcard preference or reorders the route
// registrations, this test catches it.
func TestKnowledgeRouteOrdering(t *testing.T) {
	gormDB := newTestDB(t)
	// The CRUD handler reads from the knowledge_docs table; we need
	// the schema to exist so a missing row returns 404 (not 500 from
	// "no such table"). FTS5 setup is not relevant to this test.
	if err := gormDB.AutoMigrate(&models.KnowledgeDoc{}); err != nil {
		t.Fatalf("automigrate KnowledgeDoc: %v", err)
	}
	r := gin.New()
	// Use the no-FTS path so this test runs without the fts5 build tag.
	search := NewKnowledgeSearchHandlerWithStore(nil, nil)
	search.RegisterRoutes(r)
	crud := NewKnowledgeDocHandler(gormDB)
	crud.RegisterRoutes(r)

	// Empty q on /knowledge/search returns 400 from the search handler
	// (NOT the CRUD Get handler, which would 400 only for a non-numeric
	// id — "search" would parse as a valid request and either succeed
	// or 404).
	w := do(t, r, "GET", "/knowledge/search?q=", nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("/knowledge/search: expected 400 from search handler, got %d body=%s", w.Code, w.Body.String())
	}

	// A non-numeric id on /knowledge/:id should 400 from the CRUD
	// handler. If routing regressed and "search" leaked into :id, the
	// first request above would have already failed differently.
	w = do(t, r, "GET", "/knowledge/abc", nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("/knowledge/abc: expected 400 from CRUD Get, got %d body=%s", w.Code, w.Body.String())
	}

	// A numeric id that doesn't exist should 404 from the CRUD handler
	// (proving the route resolves to Get, not Search).
	w = do(t, r, "GET", "/knowledge/9999", nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("/knowledge/9999: expected 404 from CRUD Get, got %d body=%s", w.Code, w.Body.String())
	}
}

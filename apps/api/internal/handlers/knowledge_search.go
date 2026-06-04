package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/opc/api/internal/models"
)

// KnowledgeSearchStore is the persistence contract the search endpoint
// needs. It is intentionally narrow so the handler can be exercised in
// tests with lightweight fakes, and so the production adapter can be
// backed by a raw *sql.DB (or a GORM handle) without leaking SQL
// concerns into the HTTP layer.
type KnowledgeSearchStore interface {
	Search(ctx context.Context, query string, docType string, page PageRequest) ([]models.KnowledgeDoc, int64, error)
}

// KnowledgeSearchHandler serves GET /knowledge/search. The store field
// is an interface so tests can swap in fakes; production wiring passes
// the *gormKnowledgeSearchStore adapter defined in this file.
//
// We keep the search handler separate from the CRUD KnowledgeDocHandler
// because the search surface is read-only, depends on the FTS5 virtual
// table rather than the base table, and is not part of the basic CRUD
// contract.
type KnowledgeSearchHandler struct {
	store  KnowledgeSearchStore
	logger *slog.Logger
}

// NewKnowledgeSearchHandler wires a KnowledgeSearchHandler backed by the
// given *gorm.DB. This is the primary constructor used by main.go and by
// tests that want a real (in-memory) database.
func NewKnowledgeSearchHandler(db *gorm.DB) *KnowledgeSearchHandler {
	return NewKnowledgeSearchHandlerWithStore(newGormKnowledgeSearchStore(db), nil)
}

// NewKnowledgeSearchHandlerWithStore wires a KnowledgeSearchHandler
// against the given store and logger. A nil logger falls back to
// slog.Default(). Used by tests that want to inject a fake store.
func NewKnowledgeSearchHandlerWithStore(store KnowledgeSearchStore, logger *slog.Logger) *KnowledgeSearchHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &KnowledgeSearchHandler{store: store, logger: logger}
}

// RegisterRoutes attaches the search route to the given router under
// the /knowledge namespace. The search route must be registered before
// /knowledge/:id so Gin's trie does not treat "search" as an :id value.
func (h *KnowledgeSearchHandler) RegisterRoutes(r gin.IRouter) {
	r.GET("/knowledge/search", h.Search)
}

// Search — GET /knowledge/search?q=<query>&doc_type=<optional>&limit=&offset=
//
// Response: {"items": [...], "query": "...", "total": N, "limit": N, "offset": N}
//
// The FTS5 MATCH operator requires a non-empty query string. We trim
// whitespace and treat an empty result as a 400 so callers get fast
// feedback instead of a silent empty list (which is indistinguishable
// from a real "no matches" response and a frequent source of
// integration bugs).
func (h *KnowledgeSearchHandler) Search(c *gin.Context) {
	query := strings.TrimSpace(c.Query("q"))
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "q is required"})
		return
	}

	docType := strings.TrimSpace(c.Query("doc_type"))

	page := PageRequest{Limit: defaultListPageSize, Offset: 0}
	if v := c.Query("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be a positive integer"})
			return
		}
		if n > maxListPageSize {
			n = maxListPageSize
		}
		page.Limit = n
	}
	if v := c.Query("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "offset must be a non-negative integer"})
			return
		}
		page.Offset = n
	}

	items, total, err := h.store.Search(c.Request.Context(), query, docType, page)
	if err != nil {
		// FTS5 syntax errors (malformed MATCH expression) come back as a
		// sqlite Error with code SQLITE_ERROR. The driver surfaces the
		// message text. We treat that as a 400 so users get actionable
		// feedback ("fts5: syntax error") rather than a generic 500.
		if isFTSyntaxError(err) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		h.logger.Error("search knowledge docs failed", "err", err.Error(), "request_id", c.GetString("request_id"))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to search knowledge docs"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"items":  items,
		"query":  query,
		"total":  total,
		"limit":  page.Limit,
		"offset": page.Offset,
	})
}

// isFTSyntaxError reports whether err came back from sqlite as a
// user-input syntax error against the FTS5 MATCH expression. We
// inspect the message text rather than unwrap typed errors so we
// remain driver-agnostic (both mattn/go-sqlite3 and modernc/sqlite
// return plain errors). The list of substrings is conservative —
// adding more is cheap; missing one means a 500 instead of a 400.
func isFTSyntaxError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "fts5:") ||
		strings.Contains(msg, "fts3:") ||
		strings.Contains(msg, "malformed match expression") ||
		strings.Contains(msg, "no such column") ||
		strings.Contains(msg, "unterminated string") ||
		strings.Contains(msg, "fts5: syntax error")
}

// gormKnowledgeSearchStore is the production KnowledgeSearchStore
// backed by GORM. It issues raw SQL against the FTS5 virtual table and
// joins back to the base `knowledge_docs` table to return the full
// model rows. We use db.Raw(...) because GORM's query builder does not
// understand FTS5 MATCH semantics and would otherwise try to escape the
// query string.
type gormKnowledgeSearchStore struct {
	db *gorm.DB
}

func newGormKnowledgeSearchStore(db *gorm.DB) *gormKnowledgeSearchStore {
	return &gormKnowledgeSearchStore{db: db}
}

// Search runs the FTS5 query and joins back to knowledge_docs. We sort
// by bm25 (lower = better) so the most relevant results come first;
// bm25() is built into FTS5. When the caller supplies a doc_type we
// append a WHERE clause to filter the joined row.
//
// The LIMIT/OFFSET live on the outer query, not on the FTS scan, so
// they apply to the final joined result. For Phase 1 this is the
// simpler shape; if we ever need to paginate the FTS scan itself we
// can switch to a subquery.
func (s *gormKnowledgeSearchStore) Search(ctx context.Context, query string, docType string, page PageRequest) ([]models.KnowledgeDoc, int64, error) {
	q := s.db.WithContext(withTimeout(ctx))

	// Total count uses the same MATCH expression and any doc_type
	// filter, so the value the caller sees is consistent with the
	// items slice they will page through. We count on the joined
	// result rather than the FTS table alone.
	countSQL := `SELECT count(*) FROM knowledge_docs_fts fts
		JOIN knowledge_docs kd ON kd.id = fts.rowid
		WHERE fts.knowledge_docs_fts MATCH ?`
	countArgs := []any{query}
	if docType != "" {
		countSQL += " AND kd.doc_type = ?"
		countArgs = append(countArgs, docType)
	}

	var total int64
	if err := q.Raw(countSQL, countArgs...).Scan(&total).Error; err != nil {
		return nil, 0, err
	}

	// FTS5 raises a syntax error when the user query is malformed;
	// the store layer surfaces that as-is so the handler can convert
	// it to a 400. An empty total is a legitimate "no matches" answer
	// and should be returned with an empty items slice.
	if total == 0 {
		return []models.KnowledgeDoc{}, 0, nil
	}

	sql := `SELECT kd.* FROM knowledge_docs_fts fts
		JOIN knowledge_docs kd ON kd.id = fts.rowid
		WHERE fts.knowledge_docs_fts MATCH ?`
	args := []any{query}
	if docType != "" {
		sql += " AND kd.doc_type = ?"
		args = append(args, docType)
	}
	sql += " ORDER BY bm25(fts.knowledge_docs_fts) ASC LIMIT ? OFFSET ?"
	args = append(args, page.Limit, page.Offset)

	var items []models.KnowledgeDoc
	if err := q.Raw(sql, args...).Scan(&items).Error; err != nil {
		return nil, 0, err
	}
	if items == nil {
		items = []models.KnowledgeDoc{}
	}
	return items, total, nil
}

// ErrInvalidFTSSyntax is the sentinel used by fake stores in tests to
// simulate an FTS syntax error. It is exported so tests can build
// fakes that return this exact error and assert the handler maps it
// to a 400.
var ErrInvalidFTSSyntax = errors.New("fts5: syntax error")

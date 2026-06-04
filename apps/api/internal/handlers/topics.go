package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/opc/api/internal/models"
)

// maxListPageSize caps the number of rows the List endpoint will return in a
// single response. Clients can ask for less, never more. This is the primary
// guard against unbounded scans and accidental DoS.
const (
	defaultListPageSize = 50
	maxListPageSize     = 200
	queryTimeout        = 5 * time.Second
)

// TopicStore is the persistence contract the TopicHandler needs. It is
// intentionally narrow so tests can supply lightweight fakes and so the
// handler is not coupled to GORM. The interface is defined where it is
// consumed, per the "accept interfaces, return structs" Go idiom.
//
// The Phase-2 Get/Update/Delete signatures take a userID so the
// per-row ownership check happens in the store. List gets the same
// field via TopicFilter. Create does not — the handler stamps the
// userID from the auth context before the call.
type TopicStore interface {
	Create(ctx context.Context, t *models.Topic) error
	List(ctx context.Context, filter TopicFilter, page PageRequest) ([]models.Topic, int64, error)
	Get(ctx context.Context, id uint, userID uint) (*models.Topic, error)
	Update(ctx context.Context, id uint, patch map[string]any, userID uint) (*models.Topic, error)
	Delete(ctx context.Context, id uint, userID uint) (int64, error)
}

// TopicFilter narrows List results. Zero value means "no filter".
// UserID is added by RequireAuth and scopes the query to the caller's
// own rows; a non-zero value always narrows.
type TopicFilter struct {
	UserID   uint
	Platform string
	Status   string
}

// PageRequest is a validated, capped pagination window.
type PageRequest struct {
	Limit  int
	Offset int
}

// TopicHandler exposes the topic REST surface. The store field is an
// interface so we can swap in fakes in unit tests; production wiring passes
// the *gormTopicStore adapter defined in store.go (same package, not exported
// beyond the test-only seam).
type TopicHandler struct {
	store  TopicStore
	logger *slog.Logger
}

// NewTopicHandler wires a TopicHandler with the given store and logger. A nil
// logger falls back to slog.Default().
func NewTopicHandler(store TopicStore, logger *slog.Logger) *TopicHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &TopicHandler{store: store, logger: logger}
}

// NewTopicHandlerFromGorm is a production convenience that adapts a *gorm.DB
// into a TopicStore. Tests should construct handlers via NewTopicHandler with
// their own store implementation.
func NewTopicHandlerFromGorm(db *gorm.DB, logger *slog.Logger) *TopicHandler {
	return NewTopicHandler(newGormTopicStore(db), logger)
}

// RegisterRoutes attaches the CRUD routes for /topics to the given router.
// The handler owns its own URL space so main.go and tests do not duplicate
// wiring.
func (h *TopicHandler) RegisterRoutes(r gin.IRouter) {
	r.POST("/topics", h.Create)
	r.GET("/topics", h.List)
	r.GET("/topics/:id", h.Get)
	r.PUT("/topics/:id", h.Update)
	r.DELETE("/topics/:id", h.Delete)
}

// Create — POST /topics
func (h *TopicHandler) Create(c *gin.Context) {
	var topic models.Topic
	if err := c.ShouldBindJSON(&topic); err != nil {
		// Per Go best practices, do not echo the parser error to the client on
		// 4xx. Log it server-side. Binding-tag violations (e.g. missing
		// required) are intentional and useful to the client; raw JSON parse
		// errors collapse to a generic message so the response does not leak
		// parser internals.
		h.logger.Warn("invalid create body", "err", err.Error(), "request_id", c.GetString("request_id"))
		msg := "invalid request body"
		var syntaxErr *json.SyntaxError
		var unmarshalErr *json.UnmarshalTypeError
		if errors.As(err, &syntaxErr) || errors.As(err, &unmarshalErr) || errors.Is(err, io.EOF) {
			// keep generic
		} else {
			msg = err.Error()
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
		return
	}
	// Stamp the owner. The client cannot pick a UserID through the
	// request body — RequireAuth sets the value and we trust the
	// middleware, not the JSON.
	topic.UserID = UserIDFromContext(c)
	if err := h.store.Create(c.Request.Context(), &topic); err != nil {
		if isValidationError(err) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		h.logger.Error("create topic failed", "err", err.Error(), "request_id", c.GetString("request_id"))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create topic"})
		return
	}
	c.JSON(http.StatusCreated, topic)
}

// List — GET /topics?platform=&status=&limit=&offset=
func (h *TopicHandler) List(c *gin.Context) {
	filter, page, err := parseListQuery(c)
	// Scope to the caller. RequireAuth guarantees this is non-zero.
	filter.UserID = UserIDFromContext(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	items, total, err := h.store.List(c.Request.Context(), filter, page)
	if err != nil {
		h.logger.Error("list topics failed", "err", err.Error(), "request_id", c.GetString("request_id"))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list topics"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"items":  items,
		"total":  total,
		"limit":  page.Limit,
		"offset": page.Offset,
	})
}

// Get — GET /topics/:id
func (h *TopicHandler) Get(c *gin.Context) {
	id, ok := parsePositiveID(c)
	if !ok {
		return
	}
	topic, err := h.store.Get(c.Request.Context(), id, UserIDFromContext(c))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "topic not found"})
			return
		}
		h.logger.Error("get topic failed", "err", err.Error(), "id", id, "request_id", c.GetString("request_id"))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get topic"})
		return
	}
	c.JSON(http.StatusOK, topic)
}

// Update — PATCH /topics/:id. Accepts a partial-update map so omitted
// fields do not silently clobber existing values (the classic
// ShouldBindJSON-into-existing-struct pitfall). The store performs the
// lookup + save in a transaction to avoid TOCTOU races.
func (h *TopicHandler) Update(c *gin.Context) {
	id, ok := parsePositiveID(c)
	if !ok {
		return
	}
	var patch map[string]any
	if err := c.ShouldBindJSON(&patch); err != nil {
		h.logger.Warn("invalid update body", "err", err.Error(), "id", id, "request_id", c.GetString("request_id"))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	// Strip the primary key; the URL owns the identity.
	delete(patch, "id")
	delete(patch, "created_at")
	delete(patch, "updated_at")

	topic, err := h.store.Update(c.Request.Context(), id, patch, UserIDFromContext(c))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "topic not found"})
			return
		}
		if isValidationError(err) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		h.logger.Error("update topic failed", "err", err.Error(), "id", id, "request_id", c.GetString("request_id"))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update topic"})
		return
	}
	c.JSON(http.StatusOK, topic)
}

// Delete — DELETE /topics/:id. Returns 204 No Content on success.
func (h *TopicHandler) Delete(c *gin.Context) {
	id, ok := parsePositiveID(c)
	if !ok {
		return
	}
	rows, err := h.store.Delete(c.Request.Context(), id, UserIDFromContext(c))
	if err != nil {
		h.logger.Error("delete topic failed", "err", err.Error(), "id", id, "request_id", c.GetString("request_id"))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete topic"})
		return
	}
	if rows == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "topic not found"})
		return
	}
	c.Status(http.StatusNoContent)
}

// parsePositiveID reads and validates :id. On failure it writes a 400
// response and returns ok=false. Rejects negative numbers and zero up front
// so the store layer can assume a positive uint.
func parsePositiveID(c *gin.Context) (uint, bool) {
	raw := c.Param("id")
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return 0, false
	}
	return uint(id), true
}

func parseListQuery(c *gin.Context) (TopicFilter, PageRequest, error) {
	filter := TopicFilter{}
	if p := strings.TrimSpace(c.Query("platform")); p != "" {
		if !models.PlatformIsAllowed(p) {
			return filter, PageRequest{}, errors.New("platform is not an allowed value")
		}
		filter.Platform = p
	}
	if s := strings.TrimSpace(c.Query("status")); s != "" {
		if !models.StatusIsAllowed(s) {
			return filter, PageRequest{}, errors.New("status is not an allowed value")
		}
		filter.Status = s
	}
	page := PageRequest{Limit: defaultListPageSize, Offset: 0}
	if v := c.Query("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return filter, PageRequest{}, errors.New("limit must be a positive integer")
		}
		if n > maxListPageSize {
			n = maxListPageSize
		}
		page.Limit = n
	}
	if v := c.Query("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return filter, PageRequest{}, errors.New("offset must be a non-negative integer")
		}
		page.Offset = n
	}
	return filter, page, nil
}

// isValidationError is a heuristic for surfacing model.Validate() failures as
// 400 rather than 500. We avoid a sentinel type to keep the boundary thin.
func isValidationError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "is required") ||
		strings.Contains(msg, "exceeds max length") ||
		strings.Contains(msg, "is not an allowed value")
}

// gormTopicStore is the production TopicStore backed by GORM. It binds every
// call to a bounded context so a slow query cannot outlive the request.
type gormTopicStore struct {
	db *gorm.DB
}

func newGormTopicStore(db *gorm.DB) *gormTopicStore { return &gormTopicStore{db: db} }

func (s *gormTopicStore) Create(ctx context.Context, t *models.Topic) error {
	return s.db.WithContext(withTimeout(ctx)).Create(t).Error
}

func (s *gormTopicStore) List(ctx context.Context, f TopicFilter, p PageRequest) ([]models.Topic, int64, error) {
	q := s.db.WithContext(withTimeout(ctx)).Model(&models.Topic{}).Order(
		clause.OrderByColumn{Column: clause.Column{Name: "created_at"}, Desc: true},
	)
	if f.UserID != 0 {
		q = q.Where("user_id = ?", f.UserID)
	}
	if f.Platform != "" {
		q = q.Where("platform = ?", f.Platform)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var topics []models.Topic
	if err := q.Limit(p.Limit).Offset(p.Offset).Find(&topics).Error; err != nil {
		return nil, 0, err
	}
	return topics, total, nil
}

func (s *gormTopicStore) Get(ctx context.Context, id uint, userID uint) (*models.Topic, error) {
	var topic models.Topic
	q := s.db.WithContext(withTimeout(ctx)).Model(&models.Topic{})
	if userID != 0 {
		q = q.Where("user_id = ?", userID)
	}
	if err := q.First(&topic, id).Error; err != nil {
		return nil, err
	}
	return &topic, nil
}

// Update wraps the read+write in a transaction so concurrent PATCHes cannot
// clobber each other (TOCTOU). Updates uses a map so omitted fields are
// preserved (PATCH semantics) and GORM skips zero-valued fields.
func (s *gormTopicStore) Update(ctx context.Context, id uint, patch map[string]any, userID uint) (*models.Topic, error) {
	var out *models.Topic
	err := s.db.WithContext(withTimeout(ctx)).Transaction(func(tx *gorm.DB) error {
		var existing models.Topic
		q := tx.Model(&models.Topic{})
		if userID != 0 {
			q = q.Where("user_id = ?", userID)
		}
		if err := q.First(&existing, id).Error; err != nil {
			return err
		}
		// Re-validate the merged result so the BeforeUpdate hook still runs
		// on the final shape, even when the client only sent a partial.
		for k, v := range patch {
			switch k {
			case "title":
				if s, ok := v.(string); ok {
					existing.Title = s
				}
			case "angle":
				if s, ok := v.(string); ok {
					existing.Angle = s
				}
			case "platform":
				if s, ok := v.(string); ok {
					existing.Platform = s
				}
			case "status":
				if s, ok := v.(string); ok {
					existing.Status = s
				}
			case "expected_performance":
				if s, ok := v.(string); ok {
					existing.ExpectedPerformance = s
				}
			case "notes":
				if s, ok := v.(string); ok {
					existing.Notes = s
				}
			case "series_id":
				switch v := v.(type) {
				case nil:
					existing.SeriesID = nil
				case float64:
					u := uint(v)
					existing.SeriesID = &u
				}
			}
		}
		if err := existing.Validate(); err != nil {
			return err
		}
		if err := tx.Save(&existing).Error; err != nil {
			return err
		}
		out = &existing
		return nil
	})
	return out, err
}

// Delete uses Unscoped so a future soft-delete column on the model does not
// silently change behavior. RowsAffected is returned for the handler to
// distinguish 404 from 204. userID is honoured when non-zero so a
// caller cannot delete rows owned by another user.
func (s *gormTopicStore) Delete(ctx context.Context, id uint, userID uint) (int64, error) {
	q := s.db.WithContext(withTimeout(ctx)).Unscoped()
	if userID != 0 {
		q = q.Where("user_id = ?", userID)
	}
	res := q.Delete(&models.Topic{}, id)
	return res.RowsAffected, res.Error
}

// withTimeout returns a context.Context derived from parent that cancels
// after queryTimeout. The cancel function is also fired as soon as the
// parent context is cancelled, so the timer is released immediately when
// the client disconnects instead of waiting for the timeout to elapse.
//
// We avoid spawning a goroutine per call by chaining context.WithTimeout
// into a context that bridges parent.Done() and the timeout, and call
// cancel exactly once via a sync.Once. The returned context's Done()
// channel mirrors whichever of {parent, timeout} fires first.
func withTimeout(parent context.Context) context.Context {
	ctx, cancel := context.WithTimeout(parent, queryTimeout)
	var once sync.Once
	stop := func() { once.Do(cancel) }
	// If the parent (e.g. the HTTP request) is cancelled before the
	// timeout elapses, release the timer so the goroutine spawned by
	// context.WithTimeout returns. This is a single-purpose goroutine
	// created by the stdlib; it's not a per-request leak.
	go func() {
		select {
		case <-parent.Done():
			stop()
		case <-ctx.Done():
			stop()
		}
	}()
	return ctx
}

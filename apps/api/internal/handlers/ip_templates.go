package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/opc/api/internal/models"
)

// IP template standard documents. Every IP profile gets one of each.
//
// The path prefix `ip-style-guide/` is what the knowledge_doc CRUD uses
// to group docs by IP type. We deliberately keep the same prefix the
// rest of the knowledge base already uses (see knowledge.go and the
// "ip-style" doc_type enum) so an IP template doc shows up naturally in
// the /knowledge listing and is searchable via the FTS5 index.
var ipTemplateStandardDocs = []struct {
	Path string
	Type string
	// Description doubles as a placeholder body so a freshly created
	// IP doc has something visible instead of a blank page. Teams are
	// expected to overwrite it as the IP is fleshed out.
	Description string
}{
	{Path: "ip-style-guide/style", Type: "ip-style", Description: "人设/声音/视觉 — 形象定位、视觉调性、声音标签"},
	{Path: "ip-style-guide/tone", Type: "ip-style", Description: "语气/口头禅 — 说话风格、固定句式、典型用词"},
	{Path: "ip-style-guide/topics", Type: "ip-style", Description: "种子选题 — 前 10 个推荐选题方向"},
	{Path: "ip-style-guide/anti-patterns", Type: "ip-style", Description: "不要做 — 反面案例、避雷点"},
}

// IPTemplate is the read-side projection returned by GET /ip-templates.
// It is intentionally lighter than the full KnowledgeDoc payload — the
// list view only needs the type/name/description/doc_count quartet.
type IPTemplate struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	Description string `json:"description"`
	DocCount    int    `json:"doc_count"`
}

// IPTemplateStore is the persistence contract the IPTemplateHandler
// needs. Defining the interface here (where it is consumed) keeps the
// handler decoupled from GORM and lets tests supply lightweight fakes.
//
// Phase 2 threads userID through ListAll/Create so the IP-template
// surface only sees and writes the caller's rows.
type IPTemplateStore interface {
	ListAll(ctx context.Context, userID uint) ([]models.KnowledgeDoc, error)
	Create(ctx context.Context, kd *models.KnowledgeDoc) error
}

// IPTemplateHandler exposes the IP-template REST surface. It is
// implemented as a thin orchestration layer on top of the knowledge_doc
// table — there is no separate `ip_templates` table in Phase 1, so the
// handler derives templates by grouping knowledge_docs by the IP-type
// segment of the path.
type IPTemplateHandler struct {
	store  IPTemplateStore
	logger *slog.Logger
}

// NewIPTemplateHandler wires an IPTemplateHandler backed by the given
// *gorm.DB. This is the primary constructor used by main.go and by
// tests that want a real (in-memory) database.
func NewIPTemplateHandler(db *gorm.DB) *IPTemplateHandler {
	return NewIPTemplateHandlerWithStore(newGormIPTemplateStore(db), nil)
}

// NewIPTemplateHandlerWithStore wires the handler against a custom
// store. A nil logger falls back to slog.Default().
func NewIPTemplateHandlerWithStore(store IPTemplateStore, logger *slog.Logger) *IPTemplateHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &IPTemplateHandler{store: store, logger: logger}
}

// RegisterRoutes attaches /ip-templates to the given router.
func (h *IPTemplateHandler) RegisterRoutes(r gin.IRouter) {
	r.GET("/ip-templates", h.List)
	r.POST("/ip-templates", h.Create)
}

// List — GET /ip-templates
//
// Groups all knowledge_docs by IP type. The IP type is parsed from the
// second path segment for paths under `ip-style-guide/`. Other paths
// are ignored: a SOP under `sop/foo` is not an IP template.
func (h *IPTemplateHandler) List(c *gin.Context) {
	docs, err := h.store.ListAll(c.Request.Context(), UserIDFromContext(c))
	if err != nil {
		h.logger.Error("list knowledge docs for IP templates failed", "err", err.Error(), "request_id", c.GetString("request_id"))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list IP templates"})
		return
	}

	templates := groupDocsByIPType(docs)
	sort.Slice(templates, func(i, j int) bool {
		return templates[i].Type < templates[j].Type
	})

	c.JSON(http.StatusOK, gin.H{"templates": templates})
}

// createIPTemplateRequest is the request body for POST /ip-templates.
//
// Description is optional; the handler defaults to a sensible copy
// when the caller leaves it blank.
type createIPTemplateRequest struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Create — POST /ip-templates
//
// Creates the four standard doc entries (style / tone / topics /
// anti-patterns) under the IP-type prefix. Existing docs with the same
// path are left alone — this method is intentionally idempotent at the
// path level so a re-run after a partial failure does not duplicate
// rows. The response reports the IP type/name and the doc count after
// the operation, mirroring the GET response shape so the frontend can
// re-render the IP card without a follow-up list call.
func (h *IPTemplateHandler) Create(c *gin.Context) {
	var req createIPTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("invalid create IP template body", "err", err.Error(), "request_id", c.GetString("request_id"))
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
	ipType := strings.TrimSpace(req.Type)
	name := strings.TrimSpace(req.Name)
	if ipType == "" || name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "type and name are required"})
		return
	}
	description := strings.TrimSpace(req.Description)
	if description == "" {
		description = name
	}

	now := time.Now().UTC()
	created := 0
	userID := UserIDFromContext(c)
	for _, tmpl := range ipTemplateStandardDocs {
		kd := &models.KnowledgeDoc{
			UserID:    userID,
			Title:     name + " - " + docTitleFromPath(tmpl.Path),
			Path:      "ip-style-guide/" + ipType + "/" + strings.TrimPrefix(tmpl.Path, "ip-style-guide/"),
			Content:   tmpl.Description,
			Tags:      "ip-template," + ipType,
			DocType:   tmpl.Type,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := h.store.Create(c.Request.Context(), kd); err != nil {
			h.logger.Error("create IP template doc failed", "err", err.Error(), "ip_type", ipType, "path", kd.Path, "request_id", c.GetString("request_id"))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create IP template doc"})
			return
		}
		created++
	}

	c.JSON(http.StatusCreated, IPTemplate{
		Type:        ipType,
		Name:        name,
		Description: description,
		DocCount:    created,
	})
}

// docTitleFromPath derives a human-friendly title from a path segment.
// `style` -> "风格指南"; `tone` -> "语气调性"; etc.
func docTitleFromPath(p string) string {
	switch p {
	case "style":
		return "风格指南"
	case "tone":
		return "语气调性"
	case "topics":
		return "种子选题"
	case "anti-patterns":
		return "反面清单"
	}
	return p
}

// groupDocsByIPType walks a slice of knowledge docs and projects them
// into IPTemplate rows. The IP type is the second path segment for
// paths under `ip-style-guide/`. The displayed name defaults to the IP
// type when no `ip-style-guide/<type>/name` row is present.
func groupDocsByIPType(docs []models.KnowledgeDoc) []IPTemplate {
	type acc struct {
		count        int
		name         string
		description  string
		descPriority int
	}
	bucket := make(map[string]*acc)
	const (
		// Higher priority wins. We treat the explicit "name" doc as
		// the most authoritative source for the IP's display name.
		prioName  = 4
		prioDesc  = 3
		prioOther = 1
	)
	for _, d := range docs {
		ipType, ok := ipTypeFromPath(d.Path)
		if !ok {
			continue
		}
		a, exists := bucket[ipType]
		if !exists {
			a = &acc{}
			bucket[ipType] = a
		}
		a.count++

		// The "name" doc is a special row whose content is the
		// human-readable name of the IP; everything else is content
		// the user can edit later. We accept `name` as both a path
		// segment and a doc title.
		if strings.HasSuffix(d.Path, "/name") || d.Title == "name" {
			if v := strings.TrimSpace(d.Content); v != "" {
				a.name = v
				if a.descPriority < prioName {
					a.descPriority = prioName
				}
			}
			continue
		}
		if a.descPriority < prioDesc && len(d.Content) > 0 {
			a.description = truncate(d.Content, 80)
			a.descPriority = prioDesc
			continue
		}
		if a.descPriority < prioOther && a.name == "" {
			a.name = ipType
			a.descPriority = prioOther
		}
	}

	out := make([]IPTemplate, 0, len(bucket))
	for ipType, a := range bucket {
		name := a.name
		if name == "" {
			name = ipType
		}
		desc := a.description
		if desc == "" {
			desc = name
		}
		out = append(out, IPTemplate{
			Type:        ipType,
			Name:        name,
			Description: desc,
			DocCount:    a.count,
		})
	}
	return out
}

// ipTypeFromPath extracts the IP-type segment from a doc path. Only
// paths under `ip-style-guide/<type>/...` are considered IP template
// docs; everything else is filtered out.
func ipTypeFromPath(path string) (string, bool) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", false
	}
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return "", false
	}
	if parts[0] != "ip-style-guide" {
		return "", false
	}
	t := strings.TrimSpace(parts[1])
	if t == "" {
		return "", false
	}
	return t, true
}

// truncate keeps the description short for the list view. It's a
// deliberately cheap byte-cut: descriptions are short, non-localized
// text, and the frontend renders them with line-clamp.
func truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

// gormIPTemplateStore is the production IPTemplateStore backed by
// GORM. It uses the same knowledge_docs table as the regular CRUD —
// there is no separate IP template table in Phase 1.
type gormIPTemplateStore struct {
	db *gorm.DB
}

func newGormIPTemplateStore(db *gorm.DB) *gormIPTemplateStore {
	return &gormIPTemplateStore{db: db}
}

// ListAll returns every knowledge doc owned by userID. A zero userID
// returns every row (used by tests and any future admin surface).
// The list endpoint groups in memory, which is fine at Phase 1 scale
// (hundreds of rows max).
func (s *gormIPTemplateStore) ListAll(ctx context.Context, userID uint) ([]models.KnowledgeDoc, error) {
	var items []models.KnowledgeDoc
	q := s.db.WithContext(withTimeout(ctx)).Model(&models.KnowledgeDoc{}).Order(
		clause.OrderByColumn{Column: clause.Column{Name: "created_at"}, Desc: true},
	)
	if userID != 0 {
		q = q.Where("user_id = ?", userID)
	}
	if err := q.Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

// Create inserts a single knowledge doc. The handler intentionally
// does not de-duplicate by path: the user can re-run the standard-doc
// creation, and GORM's primary key is auto-incremented so the same
// path may appear multiple times in the table. This matches the
// behavior of the existing knowledge CRUD.
func (s *gormIPTemplateStore) Create(ctx context.Context, kd *models.KnowledgeDoc) error {
	return s.db.WithContext(withTimeout(ctx)).Create(kd).Error
}

// MarshalIndentForTest is a tiny helper used by tests that need to
// debug a response body. Exported for tests only.
func MarshalIndentForTest(v any) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}

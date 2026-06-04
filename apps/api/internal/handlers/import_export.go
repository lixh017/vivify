package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/opc/api/internal/models"
)

// ExportEnvelope is the wire format for /export and the input format for
// /import. We carry an explicit version so a future migration can detect a
// file produced by a newer OPC and refuse to import it, and a timestamp
// so the user can tell at a glance when a snapshot was taken.
type ExportEnvelope struct {
	Version    int       `json:"version"`
	ExportedAt time.Time `json:"exported_at"`
	Type       string    `json:"type"`
	Items      []any     `json:"items"`
}

// importEnvelope is the request body shape for /import. We accept a
// versioned envelope (same as export) so an exported file can be piped
// straight back in, but we also accept the raw {"items": [...]} shape
// for clients that build the payload by hand.
type importEnvelope struct {
	Version    int             `json:"version"`
	ExportedAt time.Time       `json:"exported_at"`
	Type       string          `json:"type"`
	Items      []json.RawMessage `json:"items"`
}

// importErrorItem is one entry in the /import response. The client uses
// the index to correlate failures with the original payload and the
// message to display a human-readable cause. The field is named
// `message` rather than `error` so it does not collide with the
// transport-level "error" field of an HTTP failure.
type importErrorItem struct {
	Index   int    `json:"index"`
	Message string `json:"message"`
}

// importResponse is the body of a successful /import call.
type importResponse struct {
	Imported int              `json:"imported"`
	Errors   []importErrorItem `json:"errors"`
}

// ExportType / ImportType identify which collection the endpoint
// operates on. The query string on the URL selects the value; "all"
// is only meaningful for export (we then concatenate every supported
// type under a single envelope).
const (
	exportTypeAll       = "all"
	exportTypeTopic     = "topic"
	exportTypeScript    = "script"
	exportTypeContent   = "content_item"
	exportTypeKnowledge = "knowledge"

	exportVersion = 1
)

// Importer / Exporter are narrow store contracts the import-export
// handler uses. They are kept separate from the per-entity CRUD
// stores so we can pull only the fields we need for a snapshot
// without coupling the type system to all four entity models.
type exporter interface {
	exportAll(ctx context.Context) ([]any, error)
	importAll(ctx context.Context, raw []json.RawMessage) (importResult, error)
}

type importResult struct {
	Imported int
	Errors   []importErrorItem
}

// ImportExportHandler serves /export and /import. It owns the
// snapshot JSON format and the per-entity dispatch table.
type ImportExportHandler struct {
	db     *gorm.DB
	logger *slog.Logger

	// registry maps the URL query value to the actual exporter. Built
	// once at construction so the dispatch is a map lookup rather than
	// a switch at every request.
	registry map[string]exporter
}

// NewImportExportHandler wires the handler. The *gorm.DB is used to
// back the four entity stores; we keep it untyped (no per-entity
// constructor) because the export/import logic is uniform across all
// four collections.
func NewImportExportHandler(db *gorm.DB, logger *slog.Logger) *ImportExportHandler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &ImportExportHandler{db: db, logger: logger}
	h.registry = map[string]exporter{
		exportTypeTopic:     newTopicExporter(db),
		exportTypeScript:    newScriptExporter(db),
		exportTypeContent:   newContentItemExporter(db),
		exportTypeKnowledge: newKnowledgeExporter(db),
	}
	return h
}

// RegisterRoutes attaches /export and /import to the given router.
func (h *ImportExportHandler) RegisterRoutes(r gin.IRouter) {
	r.GET("/export", h.Export)
	r.POST("/import", h.Import)
}

// Export — GET /export?type=topic|script|content_item|knowledge|all
//
// Responds with a JSON attachment so the browser saves it to disk
// rather than rendering it inline. The filename embeds the local
// date so multiple exports do not silently overwrite each other on
// the user's machine.
func (h *ImportExportHandler) Export(c *gin.Context) {
	t := normalizeExportType(c.Query("type"))
	now := time.Now().UTC()

	// "all" produces an envelope per type concatenated into one
	// "items" array. Each item carries an inline "type" discriminator
	// so a later /import?type=all could dispatch on it. We keep
	// today's scope to per-type exports to avoid leaking the
	// dispatch model into the wire format prematurely.
	if t == exportTypeAll {
		items := make([]any, 0, 256)
		for _, k := range []string{exportTypeTopic, exportTypeScript, exportTypeContent, exportTypeKnowledge} {
			rows, err := h.registry[k].exportAll(c.Request.Context())
			if err != nil {
				h.logger.Error("export all failed", "type", k, "err", err.Error(), "request_id", c.GetString("request_id"))
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to export " + k})
				return
			}
			items = append(items, rows...)
		}
		respondExport(c, now, exportTypeAll, items)
		return
	}

	exp, ok := h.registry[t]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "type must be one of: topic, script, content_item, knowledge, all"})
		return
	}
	rows, err := exp.exportAll(c.Request.Context())
	if err != nil {
		h.logger.Error("export failed", "type", t, "err", err.Error(), "request_id", c.GetString("request_id"))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to export " + t})
		return
	}
	respondExport(c, now, t, rows)
}

// Import — POST /import?type=topic|script|content_item|knowledge
//
// Accepts either a full export envelope or a bare {items: [...]} body.
// Returns 200 with {imported, errors} regardless of partial failure so
// the client can show a per-row report. A 4xx is reserved for the
// envelope itself being malformed; per-row validation problems are
// reported in the response body, not as a 4xx.
func (h *ImportExportHandler) Import(c *gin.Context) {
	t := normalizeImportType(c.Query("type"))
	if t == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "type query parameter is required"})
		return
	}
	exp, ok := h.registry[t]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "type must be one of: topic, script, content_item, knowledge"})
		return
	}

	// Cap the body at 10MB. A larger payload is a misconfiguration on
	// the client side; refusing with a clear message is friendlier
	// than letting the JSON decoder allocate a multi-gigabyte buffer.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 10*1024*1024)

	var env importEnvelope
	dec := json.NewDecoder(c.Request.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&env); err != nil {
		h.logger.Warn("invalid import body", "err", err.Error(), "type", t, "request_id", c.GetString("request_id"))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
		return
	}
	if len(env.Items) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "items is empty"})
		return
	}
	if env.Version != 0 && env.Version != exportVersion {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("unsupported export version %d (expected %d)", env.Version, exportVersion),
		})
		return
	}

	res, err := exp.importAll(c.Request.Context(), env.Items)
	if err != nil {
		// importAll only returns a non-nil error for catastrophic
		// failures (e.g. context cancelled, gorm pool closed). Per-row
		// errors are inside res.Errors.
		h.logger.Error("import failed", "type", t, "err", err.Error(), "request_id", c.GetString("request_id"))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to import " + t})
		return
	}
	if res.Errors == nil {
		res.Errors = []importErrorItem{}
	}
	c.JSON(http.StatusOK, importResponse{Imported: res.Imported, Errors: res.Errors})
}

// respondExport writes the envelope as a downloadable JSON file.
func respondExport(c *gin.Context, now time.Time, t string, items []any) {
	body := ExportEnvelope{
		Version:    exportVersion,
		ExportedAt: now,
		Type:       t,
		Items:      items,
	}
	filename := fmt.Sprintf("opc-export-%s.json", now.Format("2006-01-02"))
	c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
	c.JSON(http.StatusOK, body)
}

func normalizeExportType(s string) string {
	switch s {
	case "", "all":
		return exportTypeAll
	case "topic":
		return exportTypeTopic
	case "script":
		return exportTypeScript
	case "content_item":
		return exportTypeContent
	case "knowledge":
		return exportTypeKnowledge
	}
	return s
}

func normalizeImportType(s string) string {
	switch s {
	case "topic":
		return exportTypeTopic
	case "script":
		return exportTypeScript
	case "content_item":
		return exportTypeContent
	case "knowledge":
		return exportTypeKnowledge
	}
	return ""
}

// ---- per-entity exporters ------------------------------------------------
//
// Each exporter wraps the same three responsibilities: pull all rows,
// decode a single row from the wire, and insert a single row. They
// share no state and are constructed once in NewImportExportHandler.

// topicExporter imports/exports the topics table.
type topicExporter struct{ db *gorm.DB }

func newTopicExporter(db *gorm.DB) *topicExporter { return &topicExporter{db: db} }

func (e *topicExporter) exportAll(ctx context.Context) ([]any, error) {
	var rows []models.Topic
	if err := e.db.WithContext(withTimeout(ctx)).Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]any, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i])
	}
	return out, nil
}

func (e *topicExporter) importAll(ctx context.Context, raw []json.RawMessage) (importResult, error) {
	var res importResult
	for i, msg := range raw {
		var t models.Topic
		if err := json.Unmarshal(msg, &t); err != nil {
			res.Errors = append(res.Errors, importErrorItem{Index: i, Message: "invalid topic payload: " + err.Error()})
			continue
		}
		// Reset identity so the database assigns a fresh ID. Without
		// this, an exported file re-imported on the same server would
		// collide on the primary key and fail.
		t.ID = 0
		t.CreatedAt = time.Time{}
		t.UpdatedAt = time.Time{}
		if err := e.db.WithContext(withTimeout(ctx)).Create(&t).Error; err != nil {
			res.Errors = append(res.Errors, importErrorItem{Index: i, Message: err.Error()})
			continue
		}
		res.Imported++
	}
	return res, nil
}

// scriptExporter imports/exports the scripts table.
type scriptExporter struct{ db *gorm.DB }

func newScriptExporter(db *gorm.DB) *scriptExporter { return &scriptExporter{db: db} }

func (e *scriptExporter) exportAll(ctx context.Context) ([]any, error) {
	var rows []models.Script
	if err := e.db.WithContext(withTimeout(ctx)).Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]any, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i])
	}
	return out, nil
}

func (e *scriptExporter) importAll(ctx context.Context, raw []json.RawMessage) (importResult, error) {
	var res importResult
	for i, msg := range raw {
		var s models.Script
		if err := json.Unmarshal(msg, &s); err != nil {
			res.Errors = append(res.Errors, importErrorItem{Index: i, Message: "invalid script payload: " + err.Error()})
			continue
		}
		s.ID = 0
		s.CreatedAt = time.Time{}
		s.UpdatedAt = time.Time{}
		if err := e.db.WithContext(withTimeout(ctx)).Create(&s).Error; err != nil {
			res.Errors = append(res.Errors, importErrorItem{Index: i, Message: err.Error()})
			continue
		}
		res.Imported++
	}
	return res, nil
}

// contentItemExporter imports/exports the content_items table.
type contentItemExporter struct{ db *gorm.DB }

func newContentItemExporter(db *gorm.DB) *contentItemExporter {
	return &contentItemExporter{db: db}
}

func (e *contentItemExporter) exportAll(ctx context.Context) ([]any, error) {
	var rows []models.ContentItem
	if err := e.db.WithContext(withTimeout(ctx)).Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]any, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i])
	}
	return out, nil
}

func (e *contentItemExporter) importAll(ctx context.Context, raw []json.RawMessage) (importResult, error) {
	var res importResult
	for i, msg := range raw {
		var ci models.ContentItem
		if err := json.Unmarshal(msg, &ci); err != nil {
			res.Errors = append(res.Errors, importErrorItem{Index: i, Message: "invalid content_item payload: " + err.Error()})
			continue
		}
		ci.ID = 0
		ci.CreatedAt = time.Time{}
		if err := e.db.WithContext(withTimeout(ctx)).Create(&ci).Error; err != nil {
			res.Errors = append(res.Errors, importErrorItem{Index: i, Message: err.Error()})
			continue
		}
		res.Imported++
	}
	return res, nil
}

// knowledgeExporter imports/exports the knowledge_docs table.
type knowledgeExporter struct{ db *gorm.DB }

func newKnowledgeExporter(db *gorm.DB) *knowledgeExporter { return &knowledgeExporter{db: db} }

func (e *knowledgeExporter) exportAll(ctx context.Context) ([]any, error) {
	var rows []models.KnowledgeDoc
	if err := e.db.WithContext(withTimeout(ctx)).Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]any, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i])
	}
	return out, nil
}

func (e *knowledgeExporter) importAll(ctx context.Context, raw []json.RawMessage) (importResult, error) {
	var res importResult
	for i, msg := range raw {
		var kd models.KnowledgeDoc
		if err := json.Unmarshal(msg, &kd); err != nil {
			res.Errors = append(res.Errors, importErrorItem{Index: i, Message: "invalid knowledge payload: " + err.Error()})
			continue
		}
		kd.ID = 0
		kd.CreatedAt = time.Time{}
		kd.UpdatedAt = time.Time{}
		if err := e.db.WithContext(withTimeout(ctx)).Create(&kd).Error; err != nil {
			res.Errors = append(res.Errors, importErrorItem{Index: i, Message: err.Error()})
			continue
		}
		res.Imported++
	}
	return res, nil
}

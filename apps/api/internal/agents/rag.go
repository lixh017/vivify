package agents

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"gorm.io/gorm"

	"github.com/opc/api/internal/models"
)

// ragLimit caps the number of knowledge_docs the helper pulls per
// pipeline call. Three is enough to give Claude a meaningful style
// signal without ballooning the prompt context window or the
// per-request token bill. The limit is package-private (lowercase)
// so callers cannot request more; the pipeline handler is the only
// legitimate consumer and always wants "a few" not "all".
const ragLimit = 3

// FindRelevantKnowledge runs a simple LIKE-based search over the
// knowledge_docs table for the given seed (and optionally a
// platform hint) and returns:
//
//   - content: the concatenated, header-tagged bodies of up to
//     ragLimit matched docs, ready to be injected into a Claude
//     prompt under a "STYLE REFERENCE" banner. Empty string when no
//     docs match.
//   - titles: the titles of the docs that contributed, in the same
//     order as content. Used by the pipeline response so the
//     frontend can show "this output was grounded in docs X, Y, Z"
//     for transparency (a Phase 1.5 product requirement).
//
// The query is intentionally simple — title OR content LIKE %seed%
// — because the FTS5 build is opt-in and the rest of the project
// already treats knowledge_docs as a small, hand-curated corpus
// (a few dozen rows per tenant in practice). We use LOWER() on
// both sides so case does not gate matches; SQLite supports LOWER
// out of the box and GORM does not need a custom dialect for it.
//
// The platform parameter is currently advisory: the LIKE filter
// only uses seed, but the signature is kept symmetric with the
// other AI helpers (seed + platform) so a future per-platform
// filter or doc_type weighting can be added without changing the
// pipeline call site.
//
// FindRelevantKnowledge is safe to call with a nil db — it returns
// ("", nil) so the pipeline handler can stay oblivious to the
// configuration shape during tests. The platform argument is
// trimmed; an empty platform is not an error.
//
// On a DB error the helper degrades to ("", nil) so the pipeline
// still returns a useful response (without a style-reference
// banner) — the underlying error is logged at WARN via slog so an
// operator can distinguish "no matching docs" from "the RAG lookup
// itself failed".
func FindRelevantKnowledge(ctx context.Context, db *gorm.DB, seed, platform string) (string, []string) {
	if db == nil {
		// No DB wired (test mode without gorm). Skip RAG cleanly
		// so the pipeline demo path still works.
		return "", nil
	}
	cleanedSeed := strings.TrimSpace(seed)
	if cleanedSeed == "" {
		return "", nil
	}
	// Escape SQL LIKE metacharacters in the seed. The user-supplied
	// string is not a regular expression but a LIKE pattern, so the
	// only special characters we need to escape are %, _, and \.
	// Without this, a seed like "50% off" would match every row.
	pattern := escapeLikePattern(cleanedSeed)
	like := "%" + pattern + "%"

	var docs []models.KnowledgeDoc
	// Build a single LOWER(...) LIKE LOWER(...) predicate so the
	// search is case-insensitive on both sides. GORM's placeholders
	// bind the value once and LOWER(?) re-evaluates per row, which
	// is fine at the corpus sizes we expect.
	q := db.WithContext(ctx).
		Model(&models.KnowledgeDoc{}).
		Where("LOWER(title) LIKE LOWER(?) OR LOWER(content) LIKE LOWER(?)", like, like).
		Order("updated_at DESC").
		Limit(ragLimit)
	if err := q.Find(&docs).Error; err != nil {
		// Degrade to "no RAG context" so the pipeline still returns
		// a useful response. We log the underlying error so an
		// operator can distinguish "RAG lookup failed" from
		// "no matching docs" in the operator logs.
		slog.WarnContext(ctx, "RAG lookup failed; continuing without style reference",
			"err", err.Error(),
			"seed", cleanedSeed,
			"platform", platform,
		)
		return "", nil
	}
	if len(docs) == 0 {
		return "", nil
	}

	var b strings.Builder
	titles := make([]string, 0, len(docs))
	for i, d := range docs {
		if i > 0 {
			b.WriteString("\n\n---\n\n")
		}
		fmt.Fprintf(&b, "### %s\n\n%s", strings.TrimSpace(d.Title), strings.TrimSpace(d.Content))
		titles = append(titles, d.Title)
	}
	return b.String(), titles
}

// escapeLikePattern escapes the three LIKE metacharacters
// (% _ \) so an untrusted string (a user-supplied seed) does not
// act as a wildcard. The escape character is backslash, which is
// the convention GORM/SQLite use with the default ESCAPE clause.
func escapeLikePattern(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

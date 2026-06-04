//go:build fts5

package db

import (
	"fmt"

	"gorm.io/gorm"
)

// knowledgeFTSStmts are the SQL statements that bring the FTS5 shadow table
// and its sync triggers into existence. They are idempotent: every CREATE
// uses IF NOT EXISTS so repeated migrations do not error. The FTS table is
// declared as a contentless mirror of `knowledge_docs` (`content='knowledge_docs'`,
// `content_rowid='id'`) and is kept in sync by three triggers on insert /
// delete / update, matching the contract documented in section 4.9.7 of
// the Phase 1 design spec.
//
// We use the trigram tokenizer (sqlite >= 3.34) because knowledge docs
// contain a lot of CJK text; the default unicode61 tokenizer has no
// concept of word boundaries for Han characters, so "声音调性" would not
// match a content row that contains the same characters. trigram breaks
// the input into overlapping 3-byte sequences which works equally well
// for CJK and Latin text.
var knowledgeFTSStmts = []string{
	`CREATE VIRTUAL TABLE IF NOT EXISTS knowledge_docs_fts USING fts5(
		title,
		content,
		content='knowledge_docs',
		content_rowid='id',
		tokenize='trigram'
	)`,
	`CREATE TRIGGER IF NOT EXISTS knowledge_docs_ai AFTER INSERT ON knowledge_docs BEGIN
		INSERT INTO knowledge_docs_fts(rowid, title, content) VALUES (new.id, new.title, new.content);
	END`,
	`CREATE TRIGGER IF NOT EXISTS knowledge_docs_ad AFTER DELETE ON knowledge_docs BEGIN
		INSERT INTO knowledge_docs_fts(knowledge_docs_fts, rowid, title, content) VALUES('delete', old.id, old.title, old.content);
	END`,
	`CREATE TRIGGER IF NOT EXISTS knowledge_docs_au AFTER UPDATE ON knowledge_docs BEGIN
		INSERT INTO knowledge_docs_fts(knowledge_docs_fts, rowid, title, content) VALUES('delete', old.id, old.title, old.content);
		INSERT INTO knowledge_docs_fts(rowid, title, content) VALUES (new.id, new.title, new.content);
	END`,
}

// EnsureKnowledgeFTS creates the FTS5 shadow table and sync triggers when
// they do not already exist. It is exported so test setup in other
// packages can bring the FTS surface online without duplicating the DDL
// (which has historically drifted). Production Migrate also calls it.
func EnsureKnowledgeFTS(db *gorm.DB) error {
	for _, stmt := range knowledgeFTSStmts {
		if err := db.Exec(stmt).Error; err != nil {
			return fmt.Errorf("%s: %w", firstLine(stmt), err)
		}
	}
	return nil
}

// firstLine returns the first line of s, used purely to keep error
// messages readable when the underlying DDL statement is multi-line.
func firstLine(s string) string {
	for i, r := range s {
		if r == '\n' {
			return s[:i]
		}
	}
	return s
}

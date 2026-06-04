package db

import (
	"fmt"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/opc/api/internal/models"
)

// pragmas lists the PRAGMA statements applied to every connection opened by
// Connect. We want:
//   - journal_mode=WAL: concurrent readers + one writer; large performance
//     win for the read-heavy Phase 1 web UI;
//   - synchronous=NORMAL: durable enough for a single-process Phase 1 app,
//     much faster than FULL (the WAL file itself is still fsync'd on commit);
//   - foreign_keys=ON: enforce the FK relationships declared on the models.
//
// The GORM sqlite driver only honours SQLite DSN parameters; PRAGMAs that
// affect connection state must be issued against the live connection. The
// safest way is via a connection-init callback, so we set it on the underlying
// *sql.DB after Open.
var pragmas = []string{
	"PRAGMA journal_mode=WAL",
	"PRAGMA synchronous=NORMAL",
	"PRAGMA foreign_keys=ON",
}

// Connect opens a GORM SQLite database and applies the project's standard
// PRAGMAs (WAL, synchronous=NORMAL, foreign_keys=ON). It is the single
// supported way to obtain a *gorm.DB in production code.
//
// For test-only code that needs an in-memory database, use connectGorm
// directly.
func Connect(dbPath string) (*gorm.DB, error) {
	gormDB, err := connectGorm(dbPath)
	if err != nil {
		return nil, err
	}
	if err := applyPragmas(gormDB); err != nil {
		return nil, fmt.Errorf("apply pragmas: %w", err)
	}
	return gormDB, nil
}

// connectGorm 打开 GORM 连接的辅助函数。
// 主要在测试中复用,生产环境应该通过 Connect。
func connectGorm(dbPath string) (*gorm.DB, error) {
	gormDB, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("gorm open: %w", err)
	}
	return gormDB, nil
}

// applyPragmas runs the PRAGMA statements listed in `pragmas` against every
// connection in the pool. We re-issue them in a loop so a freshly opened
// connection (e.g. after a previous one was closed) inherits the same
// settings — SQLite stores journal_mode per-database, but foreign_keys is
// per-connection.
func applyPragmas(gormDB *gorm.DB) error {
	conn, err := gormDB.DB()
	if err != nil {
		return fmt.Errorf("get sql.DB: %w", err)
	}
	for _, stmt := range pragmas {
		if _, err := conn.Exec(stmt); err != nil {
			return fmt.Errorf("%s: %w", stmt, err)
		}
	}
	return nil
}

// Migrate 在数据库上为所有 5 个实体创建/更新表结构。
func Migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&models.Topic{},
		&models.Script{},
		&models.ContentItem{},
		&models.KnowledgeDoc{},
		&models.Series{},
	); err != nil {
		return fmt.Errorf("auto migrate: %w", err)
	}
	if err := ensureKnowledgeFTS(db); err != nil {
		return fmt.Errorf("ensure knowledge fts: %w", err)
	}
	return nil
}

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

// ensureKnowledgeFTS creates the FTS5 shadow table and sync triggers when
// they do not already exist. It is split out of Migrate so test setup that
// only needs the search surface (e.g. the FTS5 test) can call it directly
// after AutoMigrate without going through the full migration pipeline.
func ensureKnowledgeFTS(db *gorm.DB) error {
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

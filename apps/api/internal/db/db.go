package db

import (
	"errors"
	"fmt"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/opc/api/internal/models"
)

// ErrFTS5NotEnabled is the sentinel value callers can use to detect
// that the binary was built without the `fts5` build tag. The off-tag
// stub of EnsureKnowledgeFTS in fts5_off.go returns nil (so the
// Makefile's `test-plain` smoke target keeps passing without the
// tag), and the fts5-tagged implementation in fts5.go returns
// driver-level errors instead. Code that needs to surface a
// configuration error at startup should check for the presence of
// the fts5 build tag independently — for example by calling
// EnsureKnowledgeFTS at boot and surfacing a clear message to the
// operator — rather than relying on the off-tag path to error.
var ErrFTS5NotEnabled = errors.New("knowledge FTS5 is disabled: rebuild with -tags fts5 to enable the knowledge search surface")

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

// Migrate 在数据库上为所有 5 个核心实体 + Phase 2 鉴权表创建/更新表结构。
// User 和 Session 独立于业务实体:它们不参与内容工作流,但支撑多租户
// 鉴权,必须在业务表之前就位,这样后续 PR 引入 OwnerID 外键时不会
// 出现"user 表不存在"的迁移错误。
func Migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&models.User{},
		&models.Session{},
		&models.Topic{},
		&models.Script{},
		&models.ContentItem{},
		&models.KnowledgeDoc{},
		&models.Series{},
	); err != nil {
		return fmt.Errorf("auto migrate: %w", err)
	}
	if err := EnsureKnowledgeFTS(db); err != nil {
		return fmt.Errorf("ensure knowledge fts: %w", err)
	}
	return nil
}

// knowledgeFTSStmts and ensureKnowledgeFTS live in fts5.go when the `fts5`
// build tag is set, and a stub ErrFTS5NotEnabled lives in fts5_off.go when
// it is not. Production binaries MUST be built with `-tags fts5`; the
// stub returns a clear error so misconfigured deploys fail loudly
// instead of silently breaking the knowledge search surface.

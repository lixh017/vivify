package db

import (
	"testing"
)

// TestMigrateCreatesTables verifies the five core tables come into existence
// after Migrate runs.
func TestMigrateCreatesTables(t *testing.T) {
	// Use a shared in-memory DB so all queries land on the same database.
	gormDB, err := connectGorm("file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}

	conn, err := gormDB.DB()
	if err != nil {
		t.Fatalf("get sql.DB from gorm: %v", err)
	}
	defer conn.Close()

	if err := Migrate(gormDB); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// users + sessions were added in Phase 2 alongside the existing five
	// content-workflow tables.
	tables := []string{
		"users", "sessions",
		"topics", "scripts", "content_items", "knowledge_docs", "series",
	}
	for _, table := range tables {
		var name string
		row := conn.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table)
		if err := row.Scan(&name); err != nil {
			t.Errorf("table %s not created: %v", table, err)
		}
	}
}

// TestConnectAppliesPragmas verifies that Connect enables the project-wide
// PRAGMA settings (WAL journal mode, NORMAL synchronous, foreign keys).
func TestConnectAppliesPragmas(t *testing.T) {
	// Use a temp file rather than :memory: because journal_mode=WAL is a
	// per-database setting and only meaningful on a persistent file.
	gormDB, err := Connect(t.TempDir() + "/opc-test.db")
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() {
		conn, _ := gormDB.DB()
		if conn != nil {
			_ = conn.Close()
		}
	})

	conn, err := gormDB.DB()
	if err != nil {
		t.Fatalf("get sql.DB: %v", err)
	}

	// journal_mode reports the active mode; we want "wal".
	var mode string
	if err := conn.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("query journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode = %q, want %q", mode, "wal")
	}

	// synchronous reports 0=OFF, 1=NORMAL, 2=FULL; we want NORMAL (1).
	var sync int
	if err := conn.QueryRow("PRAGMA synchronous").Scan(&sync); err != nil {
		t.Fatalf("query synchronous: %v", err)
	}
	if sync != 1 {
		t.Errorf("synchronous = %d, want 1 (NORMAL)", sync)
	}

	// foreign_keys is per-connection: should be ON (1).
	var fk int
	if err := conn.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil {
		t.Fatalf("query foreign_keys: %v", err)
	}
	if fk != 1 {
		t.Errorf("foreign_keys = %d, want 1 (ON)", fk)
	}
}

// TestMigrateCreatesIndexes verifies the gorm:`index` tags on the models
// produce concrete index entries in sqlite_master. We assert presence by
// name (GORM uses the convention `idx_<table>_<column>`) rather than via
// per-column queries so a refactor that renames the column doesn't silently
// drop the test's value.
func TestMigrateCreatesIndexes(t *testing.T) {
	gormDB, err := connectGorm("file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}

	conn, err := gormDB.DB()
	if err != nil {
		t.Fatalf("get sql.DB from gorm: %v", err)
	}
	defer conn.Close()

	if err := Migrate(gormDB); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	want := []string{
		"idx_topics_platform",
		"idx_topics_status",
		"idx_scripts_topic_id",
		"idx_scripts_platform",
		"idx_content_items_script_id",
		"idx_content_items_platform",
		"idx_content_items_scheduled_at",
		"idx_knowledge_docs_path",
		"idx_knowledge_docs_doc_type",
		"idx_series_ip_id",
	}

	rows, err := conn.Query(
		"SELECT name FROM sqlite_master WHERE type='index' AND tbl_name IN (?,?,?,?,?)",
		"topics", "scripts", "content_items", "knowledge_docs", "series",
	)
	if err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	defer rows.Close()

	got := make(map[string]struct{})
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan: %v", err)
		}
		// sqlite auto-creates an index named "<table>__<pk>" for each
		// primary key; that's expected, not interesting, and we don't
		// include it in `want`.
		if name == "" {
			continue
		}
		got[name] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}

	for _, name := range want {
		if _, ok := got[name]; !ok {
			t.Errorf("index %q not found (have: %v)", name, got)
		}
	}
}

// TestMigrateCreatesFTS5Surface is in migrate_fts5_test.go and is gated
// behind the `fts5` build tag, because the FTS5 virtual table can only
// be created against an SQLite engine that has the FTS5 module
// compiled in (mattn/go-sqlite3, with `-tags fts5`). The CI workflow
// runs that test with the tag set; a plain `go test ./...` skips it.

//go:build fts5

package db

import (
	"testing"
)

// TestMigrateCreatesFTS5Surface verifies that Migrate brings the FTS5
// shadow table and its three sync triggers into existence. The FTS
// table is a virtual table and shows up in sqlite_master as a
// non-standard 'table' with type text. The triggers show up as
// 'trigger' rows. We assert by name so a refactor that renames the
// objects does not silently regress.
//
// This test is build-tagged on `fts5` because the FTS5 surface can
// only be created against an SQLite engine that has the FTS5 module
// compiled in. mattn/go-sqlite3 ships FTS5 under the `fts5` build
// tag; glebarez/sqlite (pure Go) does not. The CI workflow runs this
// test with `-tags fts5`.
func TestMigrateCreatesFTS5Surface(t *testing.T) {
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
		"knowledge_docs_fts",
		"knowledge_docs_ai",
		"knowledge_docs_ad",
		"knowledge_docs_au",
	}
	for _, name := range want {
		var got string
		if err := conn.QueryRow(
			"SELECT name FROM sqlite_master WHERE name = ?",
			name,
		).Scan(&got); err != nil {
			t.Errorf("%s not created: %v", name, err)
		}
	}
}

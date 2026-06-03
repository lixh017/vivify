package db

import (
	"testing"

	_ "github.com/glebarez/sqlite"
)

func TestMigrateCreatesTables(t *testing.T) {
	// 用 GORM 的 sqlite 驱动（与 connectGorm 共享同一份 in-memory DB）
	gormDB, err := connectGorm("file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}

	// 拿到 GORM 底层的 *sql.DB 用于直接 SQL 验证
	conn, err := gormDB.DB()
	if err != nil {
		t.Fatalf("get sql.DB from gorm: %v", err)
	}
	defer conn.Close()

	if err := Migrate(gormDB); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// 验证表存在
	tables := []string{"topics", "scripts", "content_items", "knowledge_docs", "series"}
	for _, table := range tables {
		var name string
		row := conn.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table)
		if err := row.Scan(&name); err != nil {
			t.Errorf("table %s not created: %v", table, err)
		}
	}
}

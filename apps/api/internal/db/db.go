package db

import (
	"database/sql"
	"fmt"

	_ "github.com/glebarez/sqlite"
)

func Connect(dbPath string) (*sql.DB, error) {
	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if err := conn.Ping(); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}
	return conn, nil
}

// Migrate 占位——Task 3 后填充
func Migrate(db *sql.DB) error {
	return nil
}

package db

import (
	"fmt"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/opc/api/internal/models"
)

// connectGorm 打开 GORM 连接的辅助函数。
// 主要在测试中复用，生产环境可以直接用 gorm.Open。
func connectGorm(dbPath string) (*gorm.DB, error) {
	gormDB, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("gorm open: %w", err)
	}
	return gormDB, nil
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
	return nil
}

package main

import (
	"log"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/opc/api/internal/config"
	"github.com/opc/api/internal/db"
	"github.com/opc/api/internal/handlers"
)

func main() {
	cfg := config.Load()

	gormDB, err := gorm.Open(sqlite.Open(cfg.DBPath), &gorm.Config{})
	if err != nil {
		log.Fatalf("gorm open: %v", err)
	}

	if err := db.Migrate(gormDB); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	r := gin.Default()
	r.Use(handlers.RequestID())
	r.Use(cors.Default())

	r.GET("/health", handlers.Health)

	topicH := handlers.NewTopicHandlerFromGorm(gormDB, nil)
	topicH.RegisterRoutes(r)

	scriptH := handlers.NewScriptHandler(gormDB)
	scriptH.RegisterRoutes(r)

	contentItemH := handlers.NewContentItemHandler(gormDB)
	contentItemH.RegisterRoutes(r)

	knowledgeDocH := handlers.NewKnowledgeDocHandler(gormDB)
	knowledgeDocH.RegisterRoutes(r)

	seriesH := handlers.NewSeriesHandler(gormDB)
	seriesH.RegisterRoutes(r)

	log.Printf("🚀 OPC API listening on :%s", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatalf("server: %v", err)
	}
}

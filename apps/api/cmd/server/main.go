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
	r.Use(cors.Default())

	r.GET("/health", handlers.Health)

	topicH := handlers.NewTopicHandler(gormDB)
	r.POST("/topics", topicH.Create)
	r.GET("/topics", topicH.List)
	r.GET("/topics/:id", topicH.Get)
	r.PUT("/topics/:id", topicH.Update)
	r.DELETE("/topics/:id", topicH.Delete)

	log.Printf("🚀 OPC API listening on :%s", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatalf("server: %v", err)
	}
}

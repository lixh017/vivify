package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/opc/api/internal/models"
)

func setupTestRouter(t *testing.T) (*gin.Engine, *gorm.DB) {
	gin.SetMode(gin.TestMode)
	gormDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := gormDB.AutoMigrate(&models.Topic{}); err != nil {
		t.Fatal(err)
	}

	r := gin.New()
	h := NewTopicHandler(gormDB)
	r.POST("/topics", h.Create)
	r.GET("/topics", h.List)
	r.GET("/topics/:id", h.Get)
	r.PUT("/topics/:id", h.Update)
	r.DELETE("/topics/:id", h.Delete)
	return r, gormDB
}

func TestCreateTopic(t *testing.T) {
	r, _ := setupTestRouter(t)

	body := `{"title":"测试选题","angle":"反常识","platform":"抖音","status":"想法"}`
	req, _ := http.NewRequest("POST", "/topics", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d, body: %s", w.Code, w.Body.String())
	}

	var topic models.Topic
	if err := json.Unmarshal(w.Body.Bytes(), &topic); err != nil {
		t.Fatal(err)
	}
	if topic.Title != "测试选题" {
		t.Errorf("unexpected title: %s", topic.Title)
	}
	if topic.ID == 0 {
		t.Errorf("expected non-zero ID, got %d", topic.ID)
	}
}

func TestListTopics(t *testing.T) {
	r, db := setupTestRouter(t)

	// Seed 2 records
	db.Create(&models.Topic{Title: "A", Platform: "抖音", Status: "想法"})
	db.Create(&models.Topic{Title: "B", Platform: "哔哩哔哩", Status: "评估"})

	req, _ := http.NewRequest("GET", "/topics", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var topics []models.Topic
	if err := json.Unmarshal(w.Body.Bytes(), &topics); err != nil {
		t.Fatal(err)
	}
	if len(topics) != 2 {
		t.Errorf("expected 2 topics, got %d", len(topics))
	}
}

func TestGetTopic(t *testing.T) {
	r, db := setupTestRouter(t)

	created := models.Topic{Title: "Get me", Platform: "抖音", Status: "想法"}
	db.Create(&created)

	req, _ := http.NewRequest("GET", "/topics/1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}

	var topic models.Topic
	if err := json.Unmarshal(w.Body.Bytes(), &topic); err != nil {
		t.Fatal(err)
	}
	if topic.Title != "Get me" {
		t.Errorf("unexpected title: %s", topic.Title)
	}
}

func TestUpdateTopic(t *testing.T) {
	r, db := setupTestRouter(t)

	created := models.Topic{Title: "Original", Platform: "抖音", Status: "想法"}
	db.Create(&created)

	body := `{"title":"Updated","angle":"新角度","platform":"小红书","status":"评估"}`
	req, _ := http.NewRequest("PUT", "/topics/1", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}

	var topic models.Topic
	if err := json.Unmarshal(w.Body.Bytes(), &topic); err != nil {
		t.Fatal(err)
	}
	if topic.Title != "Updated" {
		t.Errorf("unexpected title: %s", topic.Title)
	}
	if topic.Status != "评估" {
		t.Errorf("unexpected status: %s", topic.Status)
	}
}

func TestDeleteTopic(t *testing.T) {
	r, db := setupTestRouter(t)

	created := models.Topic{Title: "Delete me", Platform: "抖音", Status: "想法"}
	db.Create(&created)

	req, _ := http.NewRequest("DELETE", "/topics/1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if int(resp["deleted"].(float64)) != 1 {
		t.Errorf("expected deleted=1, got %v", resp["deleted"])
	}
}

func TestListTopics_FilterByPlatform(t *testing.T) {
	r, db := setupTestRouter(t)

	db.Create(&models.Topic{Title: "A", Platform: "抖音", Status: "想法"})
	db.Create(&models.Topic{Title: "B", Platform: "哔哩哔哩", Status: "评估"})
	db.Create(&models.Topic{Title: "C", Platform: "抖音", Status: "已发布"})

	req, _ := http.NewRequest("GET", "/topics?platform=抖音", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var topics []models.Topic
	if err := json.Unmarshal(w.Body.Bytes(), &topics); err != nil {
		t.Fatal(err)
	}
	if len(topics) != 2 {
		t.Errorf("expected 2 topics with platform=抖音, got %d", len(topics))
	}
	for _, tp := range topics {
		if tp.Platform != "抖音" {
			t.Errorf("unexpected platform in result: %s", tp.Platform)
		}
	}
}

func TestListTopics_FilterByStatus(t *testing.T) {
	r, db := setupTestRouter(t)

	db.Create(&models.Topic{Title: "A", Platform: "抖音", Status: "想法"})
	db.Create(&models.Topic{Title: "B", Platform: "哔哩哔哩", Status: "评估"})
	db.Create(&models.Topic{Title: "C", Platform: "抖音", Status: "评估"})

	req, _ := http.NewRequest("GET", "/topics?status=评估", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var topics []models.Topic
	if err := json.Unmarshal(w.Body.Bytes(), &topics); err != nil {
		t.Fatal(err)
	}
	if len(topics) != 2 {
		t.Errorf("expected 2 topics with status=评估, got %d", len(topics))
	}
	for _, tp := range topics {
		if tp.Status != "评估" {
			t.Errorf("unexpected status in result: %s", tp.Status)
		}
	}
}

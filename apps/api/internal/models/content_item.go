package models

import "time"

type ContentItem struct {
	ID                 uint       `gorm:"primaryKey" json:"id"`
	ScriptID           uint       `json:"script_id"`
	Platform           string     `json:"platform"`
	ScheduledAt        *time.Time `json:"scheduled_at,omitempty"`
	PublishedAt        *time.Time `json:"published_at,omitempty"`
	PlatformURL        string     `json:"platform_url"`
	PerformanceMetrics string     `gorm:"type:text" json:"performance_metrics"` // JSON
	CreatedAt          time.Time  `json:"created_at"`
}

func (ContentItem) TableName() string { return "content_items" }

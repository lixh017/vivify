package models

import "time"

type Topic struct {
	ID                  uint      `gorm:"primaryKey" json:"id"`
	Title               string    `json:"title"`
	Angle               string    `json:"angle"`
	Platform            string    `json:"platform"` // 抖音/哔哩哔哩/小红书
	Status              string    `json:"status"`   // 想法/评估/待写/已发布
	SeriesID            *uint     `json:"series_id,omitempty"`
	ExpectedPerformance string    `json:"expected_performance"`
	Notes               string    `json:"notes"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

func (Topic) TableName() string { return "topics" }

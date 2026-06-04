package models

import "time"

type Script struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	TopicID          uint      `gorm:"index" json:"topic_id"`
	Title            string    `json:"title"`
	Content          string    `gorm:"type:text" json:"content"`
	Platform         string    `gorm:"index" json:"platform"`
	StyleFingerprint string    `gorm:"type:text" json:"style_fingerprint"` // JSON
	Tags             string    `json:"tags"`
	WordCount        int       `json:"word_count"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func (Script) TableName() string { return "scripts" }

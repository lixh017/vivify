package models

import "time"

type KnowledgeDoc struct {
	ID uint `gorm:"primaryKey" json:"id"`
	// UserID is the owner of the row (Phase 2 multi-tenancy). See Topic
	// for the backfill story and index rationale. The knowledge base is
	// shared in spirit (it is the IP's brain), but per-row ownership still
	// matters so a future "private notes" toggle can filter cleanly.
	UserID    uint      `gorm:"index" json:"user_id"`
	Title     string    `json:"title"`
	Path      string    `gorm:"index" json:"path"` // e.g. "ip-style-guide/voice"
	Content   string    `gorm:"type:text" json:"content"`
	Tags      string    `json:"tags"`
	DocType   string    `gorm:"index" json:"doc_type"` // ip-style/sop/prompt/postmortem/decision/platform-rule
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (KnowledgeDoc) TableName() string { return "knowledge_docs" }

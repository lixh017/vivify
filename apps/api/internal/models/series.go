package models

import "time"

type Series struct {
	ID uint `gorm:"primaryKey" json:"id"`
	// UserID is the owner of the row (Phase 2 multi-tenancy). See Topic
	// for the backfill story and index rationale.
	UserID      uint      `gorm:"index" json:"user_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	IPID        uint      `gorm:"index" json:"ip_id"` // Phase 2: link to IPProfile (not yet a model)
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (Series) TableName() string { return "series" }

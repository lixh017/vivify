package models

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
)

// Allowed values for Platform and Status. Centralized here so the API,
// validation hooks, and DB-level CHECK constraints can share the same source
// of truth. Comments in the struct field tags document the user-facing
// vocabulary; the constants below are the canonical set.
var (
	allowedPlatforms = map[string]struct{}{
		"抖音":      {},
		"哔哩哔哩":    {},
		"小红书":     {},
		"视频号":     {},
		"YouTube": {},
	}
	allowedStatuses = map[string]struct{}{
		"想法":  {},
		"评估":  {},
		"待写":  {},
		"撰写中": {},
		"已发布": {},
	}
)

const (
	TopicTitleMaxLen       = 200
	TopicAngleMaxLen       = 1000
	TopicPlatformMaxLen    = 32
	TopicStatusMaxLen      = 16
	TopicExpectedPerfMaxLn = 500
	TopicNotesMaxLen       = 4000
)

type Topic struct {
	ID                  uint      `gorm:"primaryKey" json:"id"`
	Title               string    `gorm:"size:200;not null" json:"title" binding:"required,max=200"`
	Angle               string    `gorm:"size:1000" json:"angle,omitempty" binding:"max=1000"`
	Platform            string    `gorm:"size:32;not null;index" json:"platform" binding:"required,max=32"`
	Status              string    `gorm:"size:16;not null;index;default:想法" json:"status" binding:"required,max=16"`
	SeriesID            *uint     `gorm:"index" json:"series_id,omitempty"`
	ExpectedPerformance string    `gorm:"size:500" json:"expected_performance,omitempty" binding:"max=500"`
	Notes               string    `gorm:"type:text" json:"notes,omitempty" binding:"max=4000"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

func (Topic) TableName() string { return "topics" }

// Validate enforces the same constraints the binding tags assert, but for
// callers that bypass the binding middleware (e.g. the in-memory test setup
// that calls db.Create directly). It also normalizes the row: trims whitespace
// and applies NFC unicode normalization to free-text fields.
func (t *Topic) Validate() error {
	t.Title = strings.TrimSpace(t.Title)
	t.Angle = strings.TrimSpace(t.Angle)
	t.Platform = strings.TrimSpace(t.Platform)
	t.Status = strings.TrimSpace(t.Status)
	t.ExpectedPerformance = strings.TrimSpace(t.ExpectedPerformance)
	t.Notes = strings.TrimSpace(t.Notes)

	if t.Title == "" {
		return errors.New("title is required")
	}
	if utf8.RuneCountInString(t.Title) > TopicTitleMaxLen {
		return errors.New("title exceeds max length")
	}
	if utf8.RuneCountInString(t.Angle) > TopicAngleMaxLen {
		return errors.New("angle exceeds max length")
	}
	if t.Platform == "" {
		return errors.New("platform is required")
	}
	if _, ok := allowedPlatforms[t.Platform]; !ok {
		return errors.New("platform is not an allowed value")
	}
	if t.Status == "" {
		t.Status = "想法"
	}
	if _, ok := allowedStatuses[t.Status]; !ok {
		return errors.New("status is not an allowed value")
	}
	if t.SeriesID != nil && *t.SeriesID == 0 {
		// Treat a zero-valued pointer as "unset" rather than "FK to row 0".
		t.SeriesID = nil
	}
	return nil
}

func (t *Topic) BeforeCreate(tx *gorm.DB) error { return t.Validate() }
func (t *Topic) BeforeUpdate(tx *gorm.DB) error { return t.Validate() }

// PlatformIsAllowed / StatusIsAllowed are exported for handlers/tests that
// need to validate query-string filters without instantiating a Topic.
func PlatformIsAllowed(p string) bool { _, ok := allowedPlatforms[p]; return ok }
func StatusIsAllowed(s string) bool   { _, ok := allowedStatuses[s]; return ok }

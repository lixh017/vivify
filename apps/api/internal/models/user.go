package models

import "time"

// User represents a single person who can sign in to the OPC web UI. Phase 1
// was single-tenant (no users); Phase 2 introduces multi-tenancy, which
// requires per-user authentication. Passwords are stored as a bcrypt hash;
// the plaintext never reaches disk. The JSON contract deliberately omits the
// hash (`json:"-"`) so the field can never leak through a marshalled
// response by accident.
type User struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Email        string    `gorm:"uniqueIndex" json:"email"`
	PasswordHash string    `json:"-"` // never expose
	Name         string    `json:"name"`

	// M1 (Sub-Spec C) — Role 区分 operator 跟 creator.
	// 老 Phase 1 用户 (role="") 由 GORM AutoMigrate 改 "operator"
	// (default 值 在 Save 时 apply).
	Role string `gorm:"default:operator;not null" json:"role"`

	// M1 (Sub-Spec C) — 软删 flag. 任何 disabled=true 的 user 调任何
	// 端点 返 403 "account disabled". 老 Phase 1 用户 disabled=false.
	Disabled bool `gorm:"default:false;not null" json:"disabled"`

	// M1 (Sub-Spec C) — 哪个 operator 创建 此 user. 0 表示 self-created
	// (老 Phase 1 operator 是 0; creator > 0 记录 是哪个 operator).
	CreatedBy uint `gorm:"default:0;not null" json:"created_by"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName pins the underlying SQLite table name so the model is
// unambiguous in logs, migrations, and the migrate tests' table list.
func (User) TableName() string { return "users" }

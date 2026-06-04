package models

import (
	"reflect"
	"testing"
)

// TestSessionTableName pins the underlying table name to "sessions".
// The auth package's tests and the db.Migrate index assertions all
// assume this name; if you change it, update them in lockstep.
func TestSessionTableName(t *testing.T) {
	s := Session{}
	if got := s.TableName(); got != "sessions" {
		t.Errorf("Session.TableName() = %q, want %q", got, "sessions")
	}
}

// TestSessionTokenUnique enforces the uniqueIndex on Token. Two
// sessions with the same Token would let a stolen-token holder log
// in as the original user without leaving a trace, so the index
// is security-relevant.
func TestSessionTokenUnique(t *testing.T) {
	gormDB := openSessionTestDB(t)
	if err := gormDB.AutoMigrate(&User{}, &Session{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}

	u := User{Email: "sess@example.com", PasswordHash: "h", Name: "n"}
	if err := gormDB.Create(&u).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}

	first := Session{UserID: u.ID, Token: "shared", ExpiresAt: nowPlusHour()}
	if err := gormDB.Create(&first).Error; err != nil {
		t.Fatalf("insert first session: %v", err)
	}

	second := Session{UserID: u.ID, Token: "shared", ExpiresAt: nowPlusHour()}
	if err := gormDB.Create(&second).Error; err == nil {
		t.Fatal("insert duplicate-token session succeeded; want unique constraint violation")
	}
}

// TestSessionUserIDIndexed guards the gorm:"index" tag on UserID.
// We check the structural tag rather than querying sqlite_master
// here so the test stays in the models package; the integration
// test in db/migrate_test.go is the source of truth for "the
// index really exists in the DB".
func TestSessionUserIDIndexed(t *testing.T) {
	tag, ok := fieldTag(Session{}, "UserID")
	if !ok {
		t.Fatal("Session has no UserID field")
	}
	if !reflect.DeepEqual(parseIndexTag(tag), "index") {
		t.Errorf("UserID tag = %q; want index annotation", tag)
	}
}

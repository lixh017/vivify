package models

import (
	"reflect"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// fieldTag returns the raw struct-tag string for the named field on
// the model struct passed in. The model value is purely a type
// carrier; the contents are irrelevant. Used by the User/Session
// unit tests to assert JSON/security-relevant tag fragments
// (e.g. `json:"-"`, `gorm:"index"`) without pulling in encoding/json
// and without coupling to GORM at test time.
func fieldTag(model any, name string) (string, bool) {
	v := reflect.ValueOf(model)
	t := v.Type()
	f, ok := t.FieldByName(name)
	if !ok {
		return "", false
	}
	return string(f.Tag), true
}

// parseIndexTag pulls the value of the `gorm` tag out of a struct
// field's tag string. We only need the index annotation here, so
// the function is intentionally narrow: if there is no gorm tag
// at all we return "".
func parseIndexTag(tag string) string {
	v := reflect.StructTag(tag)
	gormTag, ok := v.Lookup("gorm")
	if !ok {
		return ""
	}
	return gormTag
}

// openSessionTestDB is a small helper that gives the models package
// its own in-memory SQLite + AutoMigrate path. We don't depend on
// the db package's Migrate here because that would also try to
// run EnsureKnowledgeFTS, which is not relevant to the
// user/session test and would require the fts5 build tag to be set.
//
// We use a private temp file rather than shared-cache `:memory:`
// because shared in-memory DBs persist across test functions in the
// same process, which would let one test's migrations or rows leak
// into the next.
func openSessionTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	path := t.TempDir() + "/models-test.db"
	gormDB, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	return gormDB
}

// nowPlusHour is a one-line helper that returns a future timestamp
// suitable for Session.ExpiresAt in tests. The exact value doesn't
// matter; the constant is here so the intent ("future") is obvious
// at the call site.
func nowPlusHour() time.Time {
	return time.Now().Add(time.Hour)
}

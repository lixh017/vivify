//go:build !fts5

package db

import (
	"gorm.io/gorm"
)

// EnsureKnowledgeFTS is a no-op stub when the `fts5` build tag is not
// set. The FTS5 virtual table cannot be created against an SQLite
// engine built without the FTS5 module, so attempting to issue the
// DDL would always fail. The no-tag binary is intended for unit tests
// and local development; production deploys MUST be built with
// `-tags fts5` (see Dockerfile). The FTS-specific integration tests
// are themselves build-tagged, so they do not run in the no-tag
// build and do not depend on the FTS surface existing.
//
// Migrate callers that want a hard configuration error in the no-tag
// build can wrap this with a call to check for the fts5 tag at
// startup; the stub is intentionally silent so existing tests and
// the Makefile's `test-plain` smoke target keep passing without the
// fts5 tag.
func EnsureKnowledgeFTS(_ *gorm.DB) error {
	return nil
}

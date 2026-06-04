// Package docs holds the API reference assets that ship with the
// binary. The OpenAPI 3.0 document lives in openapi.json alongside
// this file; embedding it at compile time keeps the spec in
// lock-step with the binary that serves it (no risk of a stale
// spec on disk after a rollback).
package docs

import _ "embed"

//go:embed openapi.json
var openapiJSON []byte

// OpenAPIJSON returns the bytes of the embedded OpenAPI 3.0 document.
// The handler package uses this to serve the spec at /openapi.json
// without copying the byte slice into a global variable of its own.
func OpenAPIJSON() []byte {
	return openapiJSON
}

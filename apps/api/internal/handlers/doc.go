// Package handlers contains the HTTP handlers that translate incoming
// requests into operations on the OPC persistence layer. Each handler is
// responsible for its own route registration via the Register* methods so
// that wiring is owned by the handler, not by main.go.
package handlers

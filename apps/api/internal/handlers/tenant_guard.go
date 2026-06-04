package handlers

import "errors"

// ErrMissingUserID is returned by gorm* store methods that take an
// explicit userID (Get, Update, Delete) when the caller passes 0.
//
// The Phase 2 multi-tenant design assumes that every read-or-write
// path on a per-user entity runs with an authenticated user on the
// call stack. Today, the only such callers are the HTTP handlers
// mounted behind RequireAuth, so a non-zero userID is guaranteed
// before the handler runs. The store used to soften the contract
// with `if userID != 0 { Where(...) }`, which silently returned
// cross-tenant rows if a future call site (admin tool, MCP, cron)
// bypassed RequireAuth.
//
// The contract is now load-bearing: 0 is a programmer error, not
// "show me everything". Returning an error makes the bug loud at the
// boundary where it occurs — the handler that forgot to stamp the
// userID — rather than at a downstream tenant-leak test.
//
// The error is intentionally not mapped to 4xx (the request itself
// is fine; the server-side wiring is wrong). Handlers should treat
// it like any other 5xx-class failure.
var ErrMissingUserID = errors.New("handlers: store called with zero userID — RequireAuth must run first")

// requireUserID returns ErrMissingUserID when userID is 0. Use it
// at the entry of every gorm* store method that takes a userID.
// Cheap (one comparison), no allocation on the happy path.
func requireUserID(userID uint) error {
	if userID == 0 {
		return ErrMissingUserID
	}
	return nil
}

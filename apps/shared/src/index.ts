// Barrel export for the @opc/shared package. Apps that need a
// single import surface for the shared code can pull from this
// root, while narrow consumers can import the per-subpath barrels
// declared in package.json `exports` to keep their bundle slim.

export * from './api/client'
export * from './types'
export * from './utils'
export * from './hooks'
// Note: i18n has its own multi-module structure (server vs client
// vs lookup) — it is intentionally NOT re-exported from the root
// barrel because pulling in the server-only `next/headers` import
// into a client bundle would break the build. Import i18n
// explicitly via `@opc/shared/i18n` (server) or
// `@opc/shared/i18n-client` (client).

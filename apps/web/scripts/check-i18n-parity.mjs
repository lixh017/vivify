#!/usr/bin/env node
// scripts/check-i18n-parity.mjs
//
// Standalone parity check for the i18n catalogs. Exits non-zero if
// zh.json and en.json do not share the exact same key set. This is
// the CI-friendly version of the runtime `assertCatalogParity` guard
// in lib/i18n-lookup.ts; it lets CI fail fast without booting the
// Next dev server.
//
// Run: `node scripts/check-i18n-parity.mjs`
//
// Exits 0 on success, 1 on any missing or unknown key.

import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, resolve } from 'node:path'

const __dirname = dirname(fileURLToPath(import.meta.url))
const catalogDir = resolve(__dirname, '..', 'lib', 'locales')

function loadCatalog(locale) {
  const path = resolve(catalogDir, `${locale}.json`)
  return JSON.parse(readFileSync(path, 'utf8'))
}

function checkPair(zhKeys, enKeys) {
  const zhSet = new Set(zhKeys)
  const enSet = new Set(enKeys)
  const missingInEn = [...zhSet].filter((k) => !enSet.has(k))
  const missingInZh = [...enSet].filter((k) => !zhSet.has(k))
  return { missingInEn, missingInZh }
}

const zh = loadCatalog('zh')
const en = loadCatalog('en')
const zhKeys = Object.keys(zh).sort()
const enKeys = Object.keys(en).sort()

const { missingInEn, missingInZh } = checkPair(zhKeys, enKeys)

let failed = false
if (missingInEn.length > 0) {
  console.error('en.json is missing keys:')
  for (const k of missingInEn) console.error(`  - ${k}`)
  failed = true
}
if (missingInZh.length > 0) {
  console.error('zh.json is missing keys:')
  for (const k of missingInZh) console.error(`  - ${k}`)
  failed = true
}

if (failed) {
  process.exit(1)
}

console.log(
  `OK: zh.json (${zhKeys.length} keys) and en.json (${enKeys.length} keys) are in parity.`,
)

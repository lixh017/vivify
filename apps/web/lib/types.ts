// TypeScript types matching Go GORM models in apps/api/internal/models/*.
// JSON tags on the Go side use snake_case, so the wire format uses snake_case.
// We deliberately do NOT camelCase these fields: doing so would force an
// extra translation layer at every call site.

export type Topic = {
  id: number
  title: string
  angle: string
  platform: string
  status: string
  series_id?: number
  expected_performance: string
  notes: string
  created_at: string
  updated_at: string
}

export type Script = {
  id: number
  topic_id: number
  title: string
  content: string
  platform: string
  style_fingerprint: string
  tags: string
  word_count: number
  created_at: string
  updated_at: string
}

export type ContentItem = {
  id: number
  script_id: number
  platform: string
  // `null` means the date is cleared (e.g. back to 待定 on the calendar);
  // `undefined` means the field is absent. The Go server returns `null`
  // for cleared fields, so we model that explicitly rather than collapsing
  // the two states.
  scheduled_at?: string | null
  published_at?: string | null
  platform_url: string
  performance_metrics: string
  created_at: string
}

// PATCH payload for /content-items/:id. Each field is independently
// optional, and the nullable timestamp fields can be explicitly cleared
// by sending `null`. This matches the Go handler's Update semantics.
export type UpdateContentItemPatch = {
  platform?: string
  scheduled_at?: string | null
  published_at?: string | null
  platform_url?: string
  performance_metrics?: string
}

export type KnowledgeDoc = {
  id: number
  title: string
  path: string
  content: string
  tags: string
  doc_type: string
  created_at: string
  updated_at: string
}

// Paged list envelopes used by /topics and /scripts List handlers.
export type ListResponse<T> = {
  items: T[]
  total: number
  limit: number
  offset: number
}

// IP template projection returned by /ip-templates. This is a derived
// view over the knowledge_docs table, not a separate entity, so it
// carries only the list-shape quartet the UI needs.
export type IpTemplate = {
  type: string
  name: string
  description: string
  doc_count: number
}

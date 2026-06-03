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
  scheduled_at?: string
  published_at?: string
  platform_url: string
  performance_metrics: string
  created_at: string
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

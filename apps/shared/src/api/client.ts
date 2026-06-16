// Typed API client for the OPC Go backend.
//
// Transport: relative `/api/...` paths that Next.js rewrites to
// NEXT_PUBLIC_API_URL. Server-side fetches and client-side fetches both
// resolve through the same rewrite, so we never need absolute URLs here.
//
// Error handling: non-2xx responses throw an Error with a useful message
// (status + body excerpt). Callers can catch and surface to the UI.

import type {
  Topic,
  Script,
  ContentItem,
  KnowledgeDoc,
  ListResponse,
  UpdateContentItemPatch,
  IpTemplate,
  AuthUser,
  Credential,
  CreateCredentialInput,
  RotateCredentialInput,
  CredentialListResponse,
  CallLog,
  CallLogStatus,
  LogsListParams,
  LogsListResponse,
  BillingSummary,
} from '../types'

const BASE = process.env.NEXT_PUBLIC_API_URL || ''

class ApiError extends Error {
  constructor(public status: number, public body: string, public path: string) {
    super(`API ${path} failed: ${status} ${body}`)
    this.name = 'ApiError'
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}/api${path}`, {
    ...init,
    // credentials: 'include' is defensively explicit. In the current
    // same-origin setup (Next.js rewrites /api/* into the Go backend)
    // cookies are sent because the request shares an origin with the
    // page. If the API ever moves to a different origin or subdomain
    // (the CORS scenario), `include` is what tells the browser to
    // attach the opc_session cookie. Spec calls for cookie-based auth,
    // so we set it now rather than discover a regression later.
    credentials: 'include',
    headers: { 'Content-Type': 'application/json', ...init?.headers },
  })
  if (!res.ok) {
    const body = await res.text().catch(() => '')
    throw new ApiError(res.status, body, path)
  }
  // 204 No Content (Topic DELETE) — return undefined cast to T.
  if (res.status === 204) {
    return undefined as unknown as T
  }
  return res.json() as Promise<T>
}

// AiResult wraps an AI response with the demo-mode flag derived from
// the X-Demo-Mode response header. UI surfaces can branch on `demo`
// to render the "🎭 Demo Mode" badge. We intentionally keep this
// shape narrow (data + demo only) so call sites stay terse.
export type AiResult<T> = {
  data: T
  demo: boolean
}

// requestAi is request<T>'s sibling for /ai/* endpoints. It exposes
// the X-Demo-Mode response header so the UI can show the demo badge
// without re-parsing the response. The signature mirrors request<T>
// (path + init) plus an optional `demo` flag that appends ?demo=true
// when the caller wants to force demo mode regardless of server-side
// API key state.
async function requestAi<T>(
  path: string,
  init?: RequestInit,
  opts?: { demo?: boolean },
): Promise<AiResult<T>> {
  const url = opts?.demo
    ? `${path}${path.includes('?') ? '&' : '?'}demo=true`
    : path
  const res = await fetch(`${BASE}/api${url}`, {
    ...init,
    credentials: 'include',
    headers: { 'Content-Type': 'application/json', ...init?.headers },
  })
  if (!res.ok) {
    const body = await res.text().catch(() => '')
    throw new ApiError(res.status, body, url)
  }
  const demo = res.headers.get('X-Demo-Mode') === 'true'
  if (res.status === 204) {
    return { data: undefined as unknown as T, demo }
  }
  const data = (await res.json()) as T
  return { data, demo }
}

// AiCallOptions is the options bag accepted by every api.ai.* method.
// Keeping it a single shared type makes it easy to add future flags
// (e.g. `signal` for AbortController) without touching every call site.
export type AiCallOptions = {
  demo?: boolean
}

export type TopicListParams = {
  platform?: string
  status?: string
  limit?: number
  offset?: number
}

export type ScriptListParams = {
  topic_id?: number
  platform?: string
  limit?: number
  offset?: number
}

export type ContentItemListParams = {
  script_id?: number
  platform?: string
  limit?: number
  offset?: number
}

export type KnowledgeListParams = {
  doc_type?: string
  path?: string
}

export const api = {
  topics: {
    list: (params?: TopicListParams) => {
      const q = params ? new URLSearchParams(params as Record<string, string>).toString() : ''
      return request<ListResponse<Topic>>(`/topics${q ? `?${q}` : ''}`)
    },
    get: (id: number) => request<Topic>(`/topics/${id}`),
    create: (data: Partial<Topic>) =>
      request<Topic>('/topics', { method: 'POST', body: JSON.stringify(data) }),
    update: (id: number, data: Partial<Topic>) =>
      request<Topic>(`/topics/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
    delete: (id: number) =>
      request<void>(`/topics/${id}`, { method: 'DELETE' }),
  },
  scripts: {
    list: (params?: ScriptListParams) => {
      const q = params ? new URLSearchParams(params as Record<string, string>).toString() : ''
      return request<ListResponse<Script>>(`/scripts${q ? `?${q}` : ''}`)
    },
    get: (id: number) => request<Script>(`/scripts/${id}`),
    create: (data: Partial<Script>) =>
      request<Script>('/scripts', { method: 'POST', body: JSON.stringify(data) }),
    update: (id: number, data: Partial<Script>) =>
      request<Script>(`/scripts/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
    delete: (id: number) =>
      request<{ deleted: number }>(`/scripts/${id}`, { method: 'DELETE' }),
  },
  contentItems: {
    list: (params?: ContentItemListParams) => {
      const q = params ? new URLSearchParams(params as Record<string, string>).toString() : ''
      return request<ListResponse<ContentItem>>(`/content-items${q ? `?${q}` : ''}`)
    },
    get: (id: number) => request<ContentItem>(`/content-items/${id}`),
    create: (data: Partial<ContentItem>) =>
      request<ContentItem>('/content-items', {
        method: 'POST',
        body: JSON.stringify(data),
      }),
    update: (id: number, data: UpdateContentItemPatch) =>
      request<ContentItem>(`/content-items/${id}`, {
        method: 'PUT',
        body: JSON.stringify(data),
      }),
    delete: (id: number) =>
      request<{ deleted: number }>(`/content-items/${id}`, { method: 'DELETE' }),
  },
  knowledge: {
    list: (params?: KnowledgeListParams) => {
      const q = params ? new URLSearchParams(params as Record<string, string>).toString() : ''
      return request<ListResponse<KnowledgeDoc>>(`/knowledge${q ? `?${q}` : ''}`)
    },
    get: (id: number) => request<KnowledgeDoc>(`/knowledge/${id}`),
    create: (data: Partial<KnowledgeDoc>) =>
      request<KnowledgeDoc>('/knowledge', {
        method: 'POST',
        body: JSON.stringify(data),
      }),
    update: (id: number, data: Partial<KnowledgeDoc>) =>
      request<KnowledgeDoc>(`/knowledge/${id}`, {
        method: 'PUT',
        body: JSON.stringify(data),
      }),
    delete: (id: number) =>
      request<{ deleted: number }>(`/knowledge/${id}`, { method: 'DELETE' }),
  },
  series: {
    list: () => request<{ items: { id: number; name: string }[] }>('/series'),
  },
  ipTemplates: {
    list: () => request<{ templates: IpTemplate[] }>('/ip-templates'),
    create: (data: { type: string; name: string; description?: string }) =>
      request<IpTemplate>('/ip-templates', {
        method: 'POST',
        body: JSON.stringify(data),
      }),
  },
  importExport: {
    // exportURL returns the URL the browser should hit to download a
    // JSON snapshot. We expose a URL instead of a Promise<Blob> because
    // the download UX wants a same-window navigation rather than a
    // fetch — the server sends Content-Disposition: attachment and the
    // browser saves the file directly.
    exportURL: (type: ExportType) => {
      const t = type === 'all' ? 'all' : type
      return `${BASE}/api/export?type=${t}`
    },
    import: (type: ImportType, body: { items: unknown[] }) =>
      request<ImportResult>(`/import?type=${type}`, {
        method: 'POST',
        body: JSON.stringify(body),
      }),
  },
  ai: {
    // generateTopics asks Claude for N differentiated topic ideas.
    // When `options.demo` is true (or the server lacks an API key),
    // the response carries the X-Demo-Mode header and we surface it
    // via `AiResult.demo` so the UI can render the demo badge.
    generateTopics: (
      data: {
        seed: string
        platform: string
        count: number
      },
      options?: AiCallOptions,
    ) =>
      requestAi<{ topics: GeneratedTopic[] }>(
        '/ai/topics',
        { method: 'POST', body: JSON.stringify(data) },
        options,
      ),
    humanize: (data: { script: string }, options?: AiCallOptions) =>
      requestAi<{ humanized: string }>(
        '/ai/humanize',
        { method: 'POST', body: JSON.stringify(data) },
        options,
      ),
    postmortem: (contentItemId: number, options?: AiCallOptions) =>
      requestAi<{ report: string; structured: PostmortemStructured }>(
        '/ai/postmortem',
        {
          method: 'POST',
          body: JSON.stringify({ content_item_id: contentItemId }),
        },
        options,
      ),
    // score asks Claude (or the rule-based fallback, when no API key
    // is configured) to evaluate a title+script+platform combination
    // on hook/structure/platform_fit axes and return actionable
    // suggestions. The handler falls back to the rule-based scorer
    // and sets X-Demo-Mode when Claude is unavailable, so the UI
    // should branch on `demo` to render the badge.
    score: (
      data: { title: string; script: string; platform: string },
      options?: AiCallOptions,
    ) =>
      requestAi<QualityScore>(
        '/ai/score',
        { method: 'POST', body: JSON.stringify(data) },
        options,
      ),
    // platformAdapt takes a single content idea (title + angle) and
    // asks Claude to produce 抖音/哔哩哔哩/小红书 versions, plus a
    // cross-platform tip list. Like score, falls back to demo
    // responses when no API key is configured.
    platformAdapt: (
      data: { title: string; angle: string; source_platform: string },
      options?: AiCallOptions,
    ) =>
      requestAi<PlatformAdapt>(
        '/ai/platform-adapt',
        { method: 'POST', body: JSON.stringify(data) },
        options,
      ),
    // publishChecklist is rule-based (no Claude) and always
    // available. It joins a content item through to its script and
    // returns the per-rule pass/warn/fail verdicts + an overall
    // score. The frontend uses this for the pre-publish guard rail
    // on the content-item detail view.
    publishChecklist: (data: { content_item_id: number }) =>
      request<PublishChecklist>('/ai/publish-checklist', {
        method: 'POST',
        body: JSON.stringify(data),
      }),
    // deconstruct breaks a video transcript into hook / structure /
    // CTA / emotional arc / reusable patterns. Returns the full
    // parsed shape so the UI can render all sections without
    // re-parsing a raw report. Falls back to demo data when no
    // API key is configured (X-Demo-Mode header).
    deconstruct: (
      data: {
        transcript: string
        metadata?: {
          platform?: string
          duration_sec?: number
          views?: number
          likes?: number
        }
      },
      options?: AiCallOptions,
    ) =>
      requestAi<DeconstructResult>(
        '/ai/deconstruct',
        { method: 'POST', body: JSON.stringify(data) },
        options,
      ),
    // viralFormula extracts a named, reusable formula from a
    // transcript — formula name, variable slots, ordered steps,
    // example application, and variations. Called from the
    // dashboard's "提取爆款公式" action after a deconstruct.
    viralFormula: (
      data: { transcript: string },
      options?: AiCallOptions,
    ) =>
      requestAi<ViralFormulaResult>(
        '/ai/viral-formula',
        { method: 'POST', body: JSON.stringify(data) },
        options,
      ),
    // pipeline runs the full content-creation chain in ONE call:
    // topics -> script (best topic expanded) -> score ->
    // 3-platform adapt. include_knowledge gates the RAG step
    // that injects style guidance from the user's knowledge base
    // into the topic + script prompts. steps is an explicit
    // allow-list; an empty array means "all four". knowledge_used
    // is always present in the response (possibly empty) so the
    // UI can show "grounded in N docs" uniformly. Falls back to
    // the canned demo pool when no API key is configured
    // (X-Demo-Mode header).
    pipeline: (
      data: {
        seed: string
        platform: string
        include_knowledge: boolean
        steps?: PipelineStep[]
      },
      options?: AiCallOptions,
    ) =>
      requestAi<PipelineResult>(
        '/ai/pipeline',
        { method: 'POST', body: JSON.stringify(data) },
        options,
      ),
  },
  auth: {
    // login exchanges email+password for an HttpOnly session cookie
    // (opc_session). The server sets the cookie on the response; the
    // browser attaches it to subsequent /api/* requests automatically.
    login: (data: { email: string; password: string }) =>
      request<{ user: AuthUser }>('/auth/login', {
        method: 'POST',
        body: JSON.stringify(data),
      }),
    // register creates a new user account and immediately sets the
    // session cookie. The web UI uses this for first-time setup before
    // any admin user exists; admin-only invites use a different flow.
    register: (data: { email: string; password: string; name: string }) =>
      request<{ user: AuthUser }>('/auth/register', {
        method: 'POST',
        body: JSON.stringify(data),
      }),
    logout: () => request<void>('/auth/logout', { method: 'POST' }),
    // me returns the current user (resolved from the session cookie)
    // or 401. The web UI uses this to detect an unauthenticated state
    // and redirect to /login.
    me: () => request<{ user: AuthUser }>('/auth/me'),
  },
  credentials: {
    // list returns every credential owned by the caller (newest
    // first). The encrypted_key column is server-omitted; the
    // returned Credential objects carry only metadata. The 200
    // response is wrapped in {items, total} for forward-compat
    // with future limit/offset params.
    list: () => request<CredentialListResponse>('/credentials'),
    // create stores a new API key. plaintext_key is encrypted
    // server-side (AES-GCM) and never echoed back; the response
    // is the metadata of the newly inserted row.
    create: (data: CreateCredentialInput) =>
      request<Credential>('/credentials', {
        method: 'POST',
        body: JSON.stringify(data),
      }),
    // rotate swaps the encrypted key on an existing row. The
    // server keeps name/scope/created_at intact; only
    // encrypted_key and updated_at change. 404 if the row is not
    // owned by the caller.
    rotate: (id: number, data: RotateCredentialInput) =>
      request<Credential>(`/credentials/${id}/rotate`, {
        method: 'PUT',
        body: JSON.stringify(data),
      }),
    // delete hard-deletes the row and writes an audit row in
    // call_logs with skill="credential_deleted". 204 on success.
    delete: (id: number) =>
      request<void>(`/credentials/${id}`, { method: 'DELETE' }),
  },
  observability: {
    // summary returns the MCN call-observability dashboard payload
    // (today / per-day / by-skill / by-provider). The endpoint is
    // session-cookie protected and multi-tenant scoped — the
    // response contains only the calling user's rows.
    //
    // ?range=today|7d|30d controls the time window. A missing or
    // unrecognized range falls through to "7d" server-side; the
    // typed return still surfaces the resolved value in
    // `response.range` so the UI can render "近 7 天" without
    // re-deriving from the request.
    summary: (params?: ObservabilitySummaryParams) => {
      const q = params?.range ? `?range=${params.range}` : ''
      return request<ObservabilitySummary>(`/observability/summary${q}`)
    },
  },
  logs: {
    // list returns the paginated, filterable call-log surface.
    // Every filter param is optional; the server defaults to
    // the last 7 days and limit=50 when the fields are missing.
    // The response is multi-tenant scoped — a request only ever
    // returns the caller's rows. The CSV export surface is
    // reached through exportURL() below; this method is JSON-
    // only.
    list: (params?: LogsListParams) => {
      const q = params
        ? new URLSearchParams(
            Object.entries(params).reduce<Record<string, string>>(
              (acc, [k, v]) => {
                if (v === undefined || v === null) return acc
                acc[k] = String(v)
                return acc
              },
              {},
            ),
          ).toString()
        : ''
      return request<LogsListResponse>(`/logs${q ? `?${q}` : ''}`)
    },
    // get returns the full row for one call log (id, user_id,
    // skill, provider, status, latency_ms, input_tokens,
    // output_tokens, cost_cents, cost_currency, error_message,
    // created_at). 404 if the row is not owned by the caller —
    // the cross-tenant case is folded into the 404 path so the
    // response shape does not leak row existence to other
    // tenants.
    get: (id: number) => request<CallLog>(`/logs/${id}`),
    // exportURL returns the URL the browser should hit to
    // download a CSV of every row that matches the filter.
    // Exposed as a URL (not a Promise<Blob>) because the
    // download UX wants a same-window navigation rather than a
    // fetch — the server sends Content-Disposition: attachment
    // and the browser saves the file directly. limit and
    // offset in params are intentionally ignored by the server
    // (an export is always the full matching set); we still
    // accept them in the type so a caller can pass the same
    // params object it used for list() without filtering.
    exportURL: (params?: LogsListParams) => {
      const q = params
        ? new URLSearchParams(
            Object.entries(params).reduce<Record<string, string>>(
              (acc, [k, v]) => {
                if (v === undefined || v === null) return acc
                // drop pagination — export is always the full set
                if (k === 'limit' || k === 'offset') return acc
                acc[k] = String(v)
                return acc
              },
              {},
            ),
          ).toString()
        : ''
      return `${BASE}/api/logs/export.csv${q ? `?${q}` : ''}`
    },
  },
  billing: {
    // summary hits GET /api/billing/summary?month=YYYY-MM.
    // The month is optional (defaults to the current month)
    // and is encoded as a plain "YYYY-MM" string so the
    // operator can navigate to past months by typing into
    // the date input. The response carries the headline
    // KPIs plus by_skill / by_provider arrays for the
    // breakdown tables; see the BillingSummary interface
    // in the types module for the exact shape.
    summary: (month?: string) => {
      const q = month ? `?month=${encodeURIComponent(month)}` : ''
      return request<BillingSummary>(`/billing/summary${q}`)
    },
  },
  speech: {
    // synthesize hits POST /api/speech. The MiniMax TTS
    // service renders text to an mp3 file on the API box
    // and returns the saved path. Once the static handler
    // from #85 lands the returned `path` will be reachable
    // over HTTP; until then operators can fetch the file
    // off the API box directly.
    synthesize: (data: SpeechRequest) =>
      request<SpeechResponse>('/speech', {
        method: 'POST',
        body: JSON.stringify(data),
      }),
  },
  video: {
    // render hits POST /api/video. The MiniMax video
    // service generates a 5s clip and saves the result to
    // disk; the path is returned. Generation is async
    // server-side (the handler wraps the upstream polling
    // loop) and can take 1-2 minutes per clip — callers
    // should bump the fetch timeout accordingly. Like
    // speech, the path will become reachable over HTTP
    // once the static handler from #85 lands.
    render: (data: VideoRequest) =>
      request<VideoResponse>('/video', {
        method: 'POST',
        body: JSON.stringify(data),
      }),
  },
}

export type GeneratedTopic = {
  title: string
  angle: string
  expected_performance: string
  hook: string
}

export type PostmortemStructured = {
  success_factors: string[]
  reusable_patterns: string[]
  insights: string[]
  suggestions: string[]
}

// QualitySuggestion matches the {category, problem, rewrite,
// severity} shape the upgraded /ai/score endpoint returns. Each
// suggestion is a "before/after" pair: problem is what the
// heuristic flagged, rewrite is the concrete fix the user can paste
// back into their script. The legacy `message` field is gone — the
// rule-based fallback and the live Claude path now emit the same
// fields, so the frontend does not need to branch on which mode
// produced the response.
export type QualitySuggestion = {
  category: string
  problem: string
  rewrite: string
  severity: 'low' | 'medium' | 'high'
}

// QualityScore is the /ai/score wire shape. All four score fields
// are 0-100 ints; suggestions is always a non-null array (the
// handler initializes it to [] on the server side).
export type QualityScore = {
  overall_score: number
  hook_strength: number
  structure: number
  platform_fit: number
  suggestions: QualitySuggestion[]
  rewritten_hook: string
}

// PlatformAdapt is the /ai/platform-adapt wire shape. The three
// adaptation sub-objects use the Chinese platform names as keys
// (matching the server-side struct tags) so the frontend can index
// directly. cross_platform_tips is always a non-null array.
export type PlatformAdapt = {
  adaptations: {
    抖音: {
      title: string
      hashtags: string[]
      description: string
    }
    哔哩哔哩: {
      title: string
      description: string
      tags: string[]
    }
    小红书: {
      title: string
      body: string
      tags: string[]
    }
  }
  cross_platform_tips: string[]
}

// PublishCheckStatus is the pass/warn/fail enum the checklist uses.
// Modeled as a string-literal union so the UI gets exhaustiveness
// checks when it switches on the value.
export type PublishCheckStatus = 'pass' | 'warn' | 'fail'

// PublishChecklist is the /ai/publish-checklist wire shape. ready
// is true iff no check is in the fail state.
export type PublishChecklist = {
  ready: boolean
  checks: {
    name: string
    status: PublishCheckStatus
    message: string
  }[]
  overall_score: number
}

// DeconstructHook is the hook analysis block of the deconstruct
// response. Type is free-form (Claude can invent new hook
// categories); Strength is a 0-100 int normalized server-side.
export type DeconstructHook = {
  type: string
  text: string
  analysis: string
  strength: number
}

// DeconstructBeat is one beat in the structural arc. time_pct is
// 0-100; role is free-form (opening/hook/.../coda).
export type DeconstructBeat = {
  time_pct: number
  role: string
  description: string
}

// DeconstructStructure describes the video's overall structure.
// Density is 0-100 (information density per second). Pacing is
// a short Chinese phrase like "前快后慢" / "匀速".
export type DeconstructStructure = {
  pattern: string
  beats: DeconstructBeat[]
  pacing: string
  density: number
}

// DeconstructCTA describes the call-to-action: whether present,
// what kind (follow/like/share/click/buy), and where it lands.
export type DeconstructCTA = {
  present: boolean
  type: string
  placement: string
}

// DeconstructArcPoint is one sample on the emotional intensity
// curve. time_pct is 0-100; emotion is a Chinese label
// (e.g. 紧张 / 平静 / 感动); intensity is 0-100.
export type DeconstructArcPoint = {
  time_pct: number
  emotion: string
  intensity: number
}

// DeconstructPlatformFit carries per-platform recommendations
// keyed by the Chinese platform names (matching the server-side
// struct tags) so the UI can index directly.
export type DeconstructPlatformFit = {
  抖音: string
  哔哩哔哩: string
  小红书: string
}

// DeconstructResult is the full wire shape for /ai/deconstruct.
// All array fields are normalized to [] on the server, never null.
export type DeconstructResult = {
  hook: DeconstructHook
  structure: DeconstructStructure
  cta: DeconstructCTA
  emotional_arc: DeconstructArcPoint[]
  reusable_patterns: string[]
  platform_fit_notes: DeconstructPlatformFit
  overall_score: number
}

// ViralFormulaVariable is one slot in the extracted formula.
// weight is a free string (Claude picks low/medium/high); the
// UI falls back gracefully if a non-standard value comes back.
export type ViralFormulaVariable = {
  name: string
  example: string
  weight: string
}

// ViralFormulaResult is the wire shape for /ai/viral-formula.
export type ViralFormulaResult = {
  formula_name: string
  variables: ViralFormulaVariable[]
  steps: string[]
  example_application: string
  variations: string[]
}

// PipelineStep enumerates the steps the /ai/pipeline endpoint
// understands. The server drops unknown step names so a typo
// (e.g. "Scripts" with a capital S) silently skips that step —
// the UI should send the lowercase form.
export type PipelineStep = 'topics' | 'script' | 'score' | 'adapt'

// PipelineScript is the trimmed script shape the pipeline emits.
// Different from Script in lib/types.ts (which is the persisted
// row shape): the pipeline returns a generation surface, not a
// persistence surface, so it omits timestamps, ids, etc.
export type PipelineScript = {
  title: string
  content: string
  angle: string
  topic: string
}

// PipelineResult is the wire shape for /ai/pipeline. All four
// sub-fields are omitempty because the caller can gate them via
// the `steps` parameter; knowledge_used is always present (even
// when empty) so the UI can render "grounded in N docs" uniformly.
export type PipelineResult = {
  topics?: GeneratedTopic[]
  script?: PipelineScript
  score?: QualityScore
  adaptations?: PlatformAdapt
  knowledge_used: string[]
}

export { ApiError }

export type ExportType = 'topic' | 'script' | 'content_item' | 'knowledge' | 'all'
export type ImportType = 'topic' | 'script' | 'content_item' | 'knowledge'

export interface ImportResult {
  imported: number
  errors: { index: number; message: string }[]
}

// ObservabilitySummaryRange enumerates the time windows the
// /api/observability/summary endpoint understands. The values
// match the server-side SummaryRange constants — keep them in
// sync. A typo here would 400 from the server; the dashboard
// is built against this union so the type system catches drift
// at compile time.
export type ObservabilitySummaryRange = 'today' | '7d' | '30d'

// ObservabilityTodaySummary is the calendar-day rollup. Calls
// is the integer count; success_rate is a 0-1 fraction
// (e.g. 0.95 = 95%); cost_cents is the integer cent total.
// cost_currency is the ISO 4217 code stored on the dominant
// row — every row defaults to "CNY" today, but Phase 4 will
// surface multi-currency breakdowns.
export interface ObservabilityTodaySummary {
  calls: number
  success_rate: number
  cost_cents: number
  cost_currency: string
}

// ObservabilityDayPoint is one bucket in the per-day line chart
// series. date is a YYYY-MM-DD string in the server's local
// timezone (the dashboard renders it directly).
export interface ObservabilityDayPoint {
  date: string
  calls: number
  cost_cents: number
}

// ObservabilityBySkillRow is one row in the by-skill breakdown.
// p50_ms / p95_ms are server-computed nearest-rank percentiles
// over the latency_ms column. The dashboard renders them
// directly without re-computing.
export interface ObservabilityBySkillRow {
  skill: string
  calls: number
  success_rate: number
  p50_ms: number
  p95_ms: number
  cost_cents: number
}

// ObservabilityByProviderRow is one row in the by-provider
// breakdown. Latency is intentionally NOT broken out per
// provider — the dashboard renders cost + call volume only.
export interface ObservabilityByProviderRow {
  provider: string
  calls: number
  cost_cents: number
}

// ObservabilityByHourDowCell is one cell in the 7×24 activity
// heatmap (dow × hour-of-day). dow follows Go's time.Weekday
// convention (0 = Sunday, 6 = Saturday). hour is 0-23 in the
// server's local timezone. The array is SPARSE — only cells
// with at least one call are emitted; the dashboard densifies
// to a 7×24 grid by defaulting missing cells to 0. Sparse
// keeps the wire shape small (typically 30-80 cells active
// in a 7-day window) and lets the same shape work for a
// 30-day window without a payload blow-up.
export interface ObservabilityByHourDowCell {
  dow: number
  hour: number
  calls: number
}

// ObservabilitySummary is the full wire shape of
// GET /api/observability/summary. Every array field is
// always a non-null array (the server initializes to []).
// All numeric fields default to 0 when the user has no data
// in the window — the dashboard's "0 calls today" path is
// the same wire shape as the busy-day path.
export interface ObservabilitySummary {
  range: ObservabilitySummaryRange
  today: ObservabilityTodaySummary
  last_7_days: ObservabilityDayPoint[]
  by_skill: ObservabilityBySkillRow[]
  by_provider: ObservabilityByProviderRow[]
  by_hour_dow: ObservabilityByHourDowCell[]
}

// ObservabilitySummaryParams is the parameter bag for
// api.observability.summary. range is optional; the server
// defaults to "7d" when the value is missing or unrecognized.
export interface ObservabilitySummaryParams {
  range?: ObservabilitySummaryRange
}

// SpeechRequest is the body for POST /api/speech. text is
// the only required field; the rest are optional TTS tuning
// knobs (voice selects the speaker; speed / volume / pitch
// scale the corresponding acoustic dimensions; language
// boosts pronunciation; out_dir lets the operator persist
// the generated mp3 to a non-default directory).
export interface SpeechRequest {
  text: string
  voice?: string
  speed?: number
  volume?: number
  pitch?: number
  language?: string
  out_dir?: string
}

// SpeechResponse is the wire shape returned by /api/speech.
// path is the on-disk path the saved mp3 lives at (served
// over HTTP once the static handler from #85 lands). The
// remaining fields are echoed straight from the MiniMax TTS
// service so the UI can build a waveform preview without
// re-fetching the file.
export interface SpeechResponse {
  path: string
  duration_ms: number
  size_bytes: number
  sample_rate: number
  provider: string
}

// VideoRequest is the body for POST /api/video. prompt is
// the only required field. first_frame / last_frame unlock
// image-to-video (I2V) and start-end-frame (SEF) modes;
// subject_image is the subject-reference (S2V) image the
// model should anchor the subject's identity against.
export interface VideoRequest {
  prompt: string
  first_frame?: string
  last_frame?: string
  subject_image?: string
  model?: string
  out_dir?: string
}

// VideoResponse is the wire shape returned by /api/video.
// path is the on-disk path the saved mp4 lives at (served
// over HTTP once the static handler from #85 lands).
// provider is always "minimax" today — kept on the shape so
// the UI's provider-badge code does not have to branch on
// a missing key.
export interface VideoResponse {
  path: string
  provider: string
}

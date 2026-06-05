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
} from './types'

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

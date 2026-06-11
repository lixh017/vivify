'use client'

// /sandbox is the operator's playground for the AI / media skills
// exposed by the OPC backend. The page is a thin runtime adapter
// over `apps/console/lib/skills-registry.ts` — every skill the
// registry describes gets a clickable entry on the left, and the
// right pane renders a dynamic form generated from the skill's
// `params` schema. Run issues a single REST call and shows the
// raw JSON response in a code block.
//
// Layout (top-to-bottom, left-to-right):
//   1. TopBar (title only)
//   2. Two-column body
//      - Left rail (300px): skills grouped by category
//      - Right pane: dynamic form + Run + result
//
// Notes on intentional scope cuts (see task #67):
//   - No realtime / WebSocket
//   - No history
//   - No saved presets / favorites
//
// We do honor `?skill=<id>` in the URL so a deep link from
// /docs/[id] or a chat message can drop the operator straight
// onto the right skill.

import { Suspense, useEffect, useMemo, useState } from 'react'
import { useRouter, useSearchParams } from 'next/navigation'
import { TopBar } from '@/components/TopBar'
import {
  SKILLS,
  getSkill,
  type ParamSchema,
  type SkillMeta,
} from '@/lib/skills-registry'

// -------- Types --------

// `FormValue` is the per-field value the sandbox holds in state.
// The kind is determined by ParamSchema.kind, but we use a
// single union to keep the state shape uniform — text/number are
// strings until the form is submitted (so empty inputs don't
// surprise the user with a coerced "0" before they've typed
// anything).
type FormValue = string | number | boolean

type FormState = Record<string, FormValue>

type ResultState =
  | { kind: 'idle' }
  | { kind: 'loading' }
  | { kind: 'ok'; status: number; body: unknown }
  | { kind: 'error'; status: number | null; message: string }

// -------- Constants --------

const CATEGORY_ORDER: SkillMeta['category'][] = ['ai', 'media', 'admin']

const CATEGORY_LABELS: Record<SkillMeta['category'], string> = {
  ai: 'AI 生成',
  media: '媒体',
  admin: '管理',
}

const PRIMARY_SKILL_ID = SKILLS[0]?.id ?? 'ai_topics'

// -------- Helpers --------

// buildInitialValues seeds the form with each param's `default`,
// coerced to the right type for its kind. We do this once on
// skill selection so editing one field never reinitializes the
// others (no `useState` reset race).
function buildInitialValues(skill: SkillMeta): FormState {
  const state: FormState = {}
  for (const p of skill.params) {
    if (p.serverOnly) continue
    if (p.default !== undefined) {
      state[p.name] = p.default
    } else if (p.kind === 'boolean') {
      state[p.name] = false
    } else {
      // text/textarea/number/select all start as empty string
      // so the input is controlled from the first keystroke.
      state[p.name] = ''
    }
  }
  return state
}

// validateForm returns the first missing required param, or null
// when the form is valid. We keep it simple — the backend will
// do the strict re-check, this is just so the operator does not
// click Run on a form that obviously cannot succeed.
function validateForm(
  skill: SkillMeta,
  values: FormState,
): string | null {
  for (const p of skill.params) {
    if (p.serverOnly) continue
    if (!p.required) continue
    const v = values[p.name]
    if (p.kind === 'boolean') {
      // booleans always have a value (we init to false).
      continue
    }
    if (typeof v === 'string' && v.trim() === '') {
      return p.label
    }
    if (typeof v === 'number' && Number.isNaN(v)) {
      return p.label
    }
  }
  return null
}

// coerceForSubmit normalizes a FormState into the JSON body we
// send to the server. text/textarea stay strings, numbers go to
// finite numbers (NaN -> omitted so the server's own validation
// kicks in), booleans stay booleans, selects stay strings.
function coerceForSubmit(
  skill: SkillMeta,
  values: FormState,
): Record<string, unknown> {
  const body: Record<string, unknown> = {}
  for (const p of skill.params) {
    if (p.serverOnly) continue
    const v = values[p.name]
    if (p.kind === 'number') {
      if (typeof v === 'number' && Number.isFinite(v)) body[p.name] = v
      else if (typeof v === 'string' && v.trim() !== '') {
        const n = Number(v)
        if (Number.isFinite(n)) body[p.name] = n
      }
    } else if (p.kind === 'boolean') {
      body[p.name] = v === true
    } else {
      body[p.name] = typeof v === 'string' ? v : String(v ?? '')
    }
  }
  return body
}

// runSkill dispatches the form submission to the matching REST
// endpoint. We use a switch (not a dynamic path-join from the
// registry) for the reasons called out in the task brief — each
// skill has a different response shape and demo-mode flag, and
// the explicit branch is the only way to surface those honestly
// in TS without fighting the client surface.
async function runSkill(
  skill: SkillMeta,
  body: Record<string, unknown>,
): Promise<{ status: number; data: unknown }> {
  const res = await fetch(`/api${skill.path}`, {
    method: skill.method,
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body:
      skill.method === 'GET' || skill.method === 'DELETE'
        ? undefined
        : JSON.stringify(body),
  })
  let data: unknown = null
  if (res.status !== 204) {
    const text = await res.text()
    try {
      data = text.length > 0 ? JSON.parse(text) : null
    } catch {
      data = text
    }
  }
  return { status: res.status, data }
}

// -------- Subcomponents --------

// SkillListItem is one row in the left rail. The active row
// gets a coral left border so the operator can always tell which
// skill the right pane is bound to, even at a glance.
function SkillListItem({
  skill,
  active,
  onSelect,
}: {
  skill: SkillMeta
  active: boolean
  onSelect: (id: string) => void
}) {
  return (
    <button
      type="button"
      onClick={() => onSelect(skill.id)}
      className={
        'w-full text-left px-4 py-2.5 border-l-2 transition-colors ' +
        (active
          ? 'border-claude-coral bg-claude-surface-soft'
          : 'border-transparent hover:bg-claude-canvas')
      }
    >
      <div className="text-sm text-claude-ink font-medium truncate">
        {skill.name}
      </div>
      <div className="mt-0.5 text-[11px] text-claude-muted-soft truncate">
        {skill.costCentsHint}
      </div>
    </button>
  )
}

// FieldRow renders one form control for one ParamSchema entry.
// We use a small dispatch on `kind` rather than a polymorphic
// component because the four kinds each have only a handful of
// props; the dispatch is easier to read at a glance.
function FieldRow({
  param,
  value,
  onChange,
}: {
  param: ParamSchema
  value: FormValue
  onChange: (next: FormValue) => void
}) {
  const labelEl = (
    <label
      htmlFor={`f-${param.name}`}
      className="text-sm text-claude-ink font-medium flex items-center gap-1"
    >
      {param.label}
      {param.required && (
        <span className="text-claude-coral" aria-label="必填">
          *
        </span>
      )}
    </label>
  )
  const helpEl = param.help ? (
    <p className="text-[11px] text-claude-muted-soft mt-1">{param.help}</p>
  ) : null
  const baseInput =
    'mt-1.5 w-full px-3 py-2 text-sm bg-white border border-claude-hairline rounded-md text-claude-ink placeholder:text-claude-muted-soft focus:outline-none focus:ring-2 focus:ring-claude-coral/30 focus:border-claude-coral transition-colors'

  let control: React.ReactNode
  if (param.kind === 'text') {
    control = (
      <input
        id={`f-${param.name}`}
        type="text"
        className={baseInput}
        value={typeof value === 'string' ? value : ''}
        onChange={(e) => onChange(e.target.value)}
      />
    )
  } else if (param.kind === 'textarea') {
    control = (
      <textarea
        id={`f-${param.name}`}
        rows={4}
        className={baseInput}
        value={typeof value === 'string' ? value : ''}
        onChange={(e) => onChange(e.target.value)}
      />
    )
  } else if (param.kind === 'number') {
    control = (
      <input
        id={`f-${param.name}`}
        type="number"
        className={baseInput}
        value={typeof value === 'number' || typeof value === 'string' ? String(value) : ''}
        onChange={(e) => onChange(e.target.value)}
      />
    )
  } else if (param.kind === 'select') {
    control = (
      <select
        id={`f-${param.name}`}
        className={baseInput}
        value={typeof value === 'string' ? value : ''}
        onChange={(e) => onChange(e.target.value)}
      >
        {(param.options ?? []).map((opt) => (
          <option key={opt.value} value={opt.value}>
            {opt.label}
          </option>
        ))}
      </select>
    )
  } else {
    // boolean
    control = (
      <label className="mt-2 inline-flex items-center gap-2 text-sm text-claude-body">
        <input
          id={`f-${param.name}`}
          type="checkbox"
          checked={value === true}
          onChange={(e) => onChange(e.target.checked)}
          className="w-4 h-4 rounded border-claude-hairline text-claude-coral focus:ring-claude-coral/30"
        />
        启用
      </label>
    )
  }

  return (
    <div>
      {labelEl}
      {control}
      {helpEl}
    </div>
  )
}

// ResultPanel renders the four result states. The success state
// uses <pre> + JSON.stringify(_, null, 2) so the operator gets a
// readable (and copyable) dump without us having to walk the
// shape ourselves. The error state mirrors the claude-error
// style used by /observability and /credentials so the surface
// stays uniform.
function ResultPanel({ result }: { result: ResultState }) {
  if (result.kind === 'idle') {
    return (
      <div className="text-sm text-claude-muted-soft py-2">
        点击 Run 后, 后端响应会显示在这里 (raw JSON)。
      </div>
    )
  }
  if (result.kind === 'loading') {
    return (
      <div className="text-sm text-claude-muted flex items-center gap-2 py-2">
        <span
          className="inline-block w-3 h-3 border-2 border-claude-coral/30 border-t-claude-coral rounded-full animate-spin"
          aria-hidden="true"
        />
        调用中…
      </div>
    )
  }
  if (result.kind === 'ok') {
    // Detect media responses and render them inline so the
    // operator can immediately see / hear the result without
    // copy-pasting the URL into a browser tab. The frontend
    // doesn't know the server's host:port, so we assume the
    // /static mount is on the same origin as the console —
    // true today because Next.js dev / standalone / nginx
    // proxy all keep them on the same origin.
    const body = result.body as Record<string, unknown> | null
    const imageURL = pickStringField(body, ['image_url', 'imageUrl'])
    const audioURL = pickStringField(body, ['path', 'audio_url', 'audioUrl'])
    const videoURL = pickStringField(body, ['path', 'video_url', 'videoUrl'])
    return (
      <div className="space-y-3">
        <div className="text-[11px] uppercase tracking-eyebrow text-claude-success">
          200 OK
        </div>
        {imageURL && imageURL.startsWith('/static/') && (
          <div>
            <div className="text-[11px] uppercase tracking-eyebrow text-claude-muted mb-1">预览</div>
            <img
              src={imageURL}
              alt="cover preview"
              className="max-h-64 rounded-md border border-claude-hairline-soft"
            />
            <div className="mt-1 text-xs text-claude-muted break-all">{imageURL}</div>
          </div>
        )}
        {audioURL && audioURL.startsWith('/static/') && audioURL.endsWith('.mp3') && (
          <div>
            <div className="text-[11px] uppercase tracking-eyebrow text-claude-muted mb-1">试听</div>
            <audio
              src={audioURL}
              controls
              className="w-full"
            />
            <div className="mt-1 text-xs text-claude-muted break-all">{audioURL}</div>
          </div>
        )}
        {videoURL && videoURL.startsWith('/static/') && videoURL.endsWith('.mp4') && (
          <div>
            <div className="text-[11px] uppercase tracking-eyebrow text-claude-muted mb-1">视频</div>
            <video src={videoURL} controls className="max-h-64 rounded-md" />
            <div className="mt-1 text-xs text-claude-muted break-all">{videoURL}</div>
          </div>
        )}
        <pre className="text-xs font-mono leading-relaxed text-claude-body bg-claude-surface-soft border border-claude-hairline-soft rounded-md p-3 overflow-auto max-h-96 whitespace-pre-wrap break-words">
          {JSON.stringify(result.body, null, 2)}
        </pre>
      </div>
    )
  }
  return (
    <div className="bg-claude-canvas border border-claude-error/30 rounded-lg px-4 py-3 flex items-start gap-3">
      <div className="text-claude-error mt-0.5">!</div>
      <div className="flex-1 min-w-0">
        <p className="text-sm text-claude-ink">
          调用失败
          {result.status !== null && (
            <span className="ml-2 text-xs text-claude-muted">
              HTTP {result.status}
            </span>
          )}
        </p>
        <p className="text-xs text-claude-muted mt-0.5 break-all whitespace-pre-wrap">
          {result.message}
        </p>
      </div>
    </div>
  )
}

// -------- Page --------

// SandboxPageInner reads the `?skill=<id>` query string. We keep
// the page itself thin and let a Suspense boundary wrap the
// inner component (useSearchParams requires it under Next 14).
function SandboxPageInner() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const initialId = searchParams.get('skill') ?? PRIMARY_SKILL_ID
  const initialSkill = getSkill(initialId) ?? getSkill(PRIMARY_SKILL_ID)
  if (!initialSkill) {
    // Should be unreachable: the registry always has at least
    // one skill. Render a graceful empty state so the build
    // stays strict.
    return (
      <>
        <TopBar title="沙盒测试" />
        <div className="p-6 text-sm text-claude-muted">
          registry 是空的, 无法渲染。
        </div>
      </>
    )
  }
  return (
    <SandboxView
      initialSkill={initialSkill}
      onSkillChange={(id) => {
        // Keep the URL in sync so the page is deep-linkable.
        // We use replace() to avoid back-button pollution while
        // the operator clicks around the rail.
        const next = new URLSearchParams(Array.from(searchParams.entries()))
        next.set('skill', id)
        router.replace(`/sandbox?${next.toString()}`)
      }}
    />
  )
}

function SandboxView({
  initialSkill,
  onSkillChange,
}: {
  initialSkill: SkillMeta
  onSkillChange: (id: string) => void
}) {
  const [skill, setSkill] = useState<SkillMeta>(initialSkill)
  const [values, setValues] = useState<FormState>(() =>
    buildInitialValues(initialSkill),
  )
  const [result, setResult] = useState<ResultState>({ kind: 'idle' })

  // Reseed the form when the selected skill changes. We
  // intentionally use the skill.id as the dep — switching to the
  // same skill (impossible, but defensive) is a no-op.
  useEffect(() => {
    setValues(buildInitialValues(skill))
    setResult({ kind: 'idle' })
  }, [skill])

  const grouped = useMemo(() => {
    const out: Record<SkillMeta['category'], SkillMeta[]> = {
      ai: [],
      media: [],
      admin: [],
    }
    for (const s of SKILLS) {
      out[s.category].push(s)
    }
    return out
  }, [])

  function handleSelect(id: string) {
    const next = getSkill(id)
    if (!next) return
    setSkill(next)
    onSkillChange(id)
  }

  function handleFieldChange(name: string, value: FormValue) {
    setValues((prev) => ({ ...prev, [name]: value }))
  }

  async function handleRun() {
    const missing = validateForm(skill, values)
    if (missing) {
      setResult({
        kind: 'error',
        status: null,
        message: `请填写必填字段: ${missing}`,
      })
      return
    }
    setResult({ kind: 'loading' })
    const body = coerceForSubmit(skill, values)
    try {
      const { status, data } = await runSkill(skill, body)
      if (status >= 200 && status < 300) {
        setResult({ kind: 'ok', status, body: data })
      } else {
        setResult({
          kind: 'error',
          status,
          message:
            typeof data === 'string' && data.length > 0
              ? data
              : `HTTP ${status}`,
        })
      }
    } catch (err: unknown) {
      setResult({
        kind: 'error',
        status: null,
        message: err instanceof Error ? err.message : '调用失败',
      })
    }
  }

  return (
    <>
      <TopBar title="沙盒测试" />
      <div className="flex flex-1 min-h-0">
        {/* Left rail: skill list, grouped by category */}
        <aside className="w-[300px] shrink-0 border-r border-claude-hairline bg-white overflow-y-auto">
          <div className="p-4 border-b border-claude-hairline-soft">
            <h2 className="text-sm font-medium text-claude-ink">Skills</h2>
            <p className="mt-0.5 text-[11px] text-claude-muted-soft">
              按 category 分组 · 点击切换
            </p>
          </div>
          {CATEGORY_ORDER.map((cat) => {
            const items = grouped[cat]
            if (items.length === 0) return null
            return (
              <div key={cat} className="py-2">
                <div className="px-4 py-1.5 text-[11px] uppercase tracking-eyebrow text-claude-muted">
                  {CATEGORY_LABELS[cat]}
                </div>
                {items.map((s) => (
                  <SkillListItem
                    key={s.id}
                    skill={s}
                    active={s.id === skill.id}
                    onSelect={handleSelect}
                  />
                ))}
              </div>
            )
          })}
        </aside>

        {/* Right pane: dynamic form + result */}
        <main className="flex-1 min-w-0 overflow-y-auto">
          <div className="p-6 space-y-5 max-w-3xl">
            {/* Skill header */}
            <div>
              <div className="flex items-center gap-2">
                <h2 className="text-base font-medium text-claude-ink">
                  {skill.name}
                </h2>
                <span className="text-[11px] font-mono px-1.5 py-0.5 rounded bg-claude-surface-soft text-claude-body">
                  {skill.method} {skill.path}
                </span>
              </div>
              <p className="mt-1 text-sm text-claude-muted">
                {skill.description}
              </p>
              <p className="mt-1 text-[11px] text-claude-muted-soft">
                限流: {skill.rateLimit} · 成本: {skill.costCentsHint}
              </p>
            </div>

            {/* Form */}
            <div className="bg-white border border-claude-hairline rounded-lg p-5 shadow-claude-soft space-y-4">
              {skill.params.filter((p) => !p.serverOnly).length === 0 ? (
                <p className="text-sm text-claude-muted">
                  这个 skill 没有入参, 直接点 Run 就行。
                </p>
              ) : (
                skill.params
                  .filter((p) => !p.serverOnly)
                  .map((p) => (
                    <FieldRow
                      key={p.name}
                      param={p}
                      value={values[p.name] ?? ''}
                      onChange={(v) => handleFieldChange(p.name, v)}
                    />
                  ))
              )}
              <div className="pt-2 flex items-center gap-3">
                <button
                  type="button"
                  onClick={() => void handleRun()}
                  disabled={result.kind === 'loading'}
                  className="inline-flex items-center gap-2 px-4 py-2 text-sm bg-claude-coral text-claude-on-primary rounded-md hover:bg-claude-coral-active transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  {result.kind === 'loading' ? (
                    <>
                      <span
                        className="inline-block w-3 h-3 border-2 border-white/40 border-t-white rounded-full animate-spin"
                        aria-hidden="true"
                      />
                      运行中…
                    </>
                  ) : (
                    'Run'
                  )}
                </button>
                <button
                  type="button"
                  onClick={() => {
                    setValues(buildInitialValues(skill))
                    setResult({ kind: 'idle' })
                  }}
                  disabled={result.kind === 'loading'}
                  className="px-3 py-2 text-sm text-claude-muted hover:text-claude-ink hover:bg-claude-surface-soft rounded-md transition-colors disabled:opacity-50"
                >
                  重置
                </button>
              </div>
            </div>

            {/* Result */}
            <div className="bg-white border border-claude-hairline rounded-lg p-5 shadow-claude-soft">
              <div className="flex items-center justify-between mb-3">
                <h3 className="text-sm font-medium text-claude-ink">结果</h3>
                <span className="text-[11px] text-claude-muted-soft">
                  raw JSON
                </span>
              </div>
              <ResultPanel result={result} />
            </div>
          </div>
        </main>
      </div>
    </>
  )
}

export default function SandboxPage() {
  return (
    <Suspense
      fallback={
        <>
          <TopBar title="沙盒测试" />
          <div className="p-6 text-sm text-claude-muted">加载中…</div>
        </>
      }
    >
      <SandboxPageInner />
    </Suspense>
  )
}

// pickStringField returns the first non-empty string field from
// the response body, looking up each name in order. Used by
// ResultPanel to detect media responses without hard-coding
// one wire shape — the speech handler returns "path", the
// cover handler returns "image_url", and a future video
// handler may return either depending on the iteration.
function pickStringField(
  body: Record<string, unknown> | null,
  names: string[],
): string | null {
  if (!body) return null
  for (const name of names) {
    const v = body[name]
    if (typeof v === 'string' && v.length > 0) return v
  }
  return null
}

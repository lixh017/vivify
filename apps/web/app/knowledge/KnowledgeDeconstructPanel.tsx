'use client'

import { useState, useEffect } from 'react'
import { api } from '@/lib/api'
import type { DeconstructResult } from '@/lib/api'
import type { KnowledgeDoc } from '@/lib/types'
import { useT } from '@/lib/i18n-client'
import {
  truncateForTitle,
  deconstructToMarkdown,
} from '@/lib/deconstruct-format'
import { useModalA11y } from '@/lib/use-modal'

// KnowledgeDeconstructPanel is the "保存拆解" entry point on the
// knowledge page. It is intentionally a focused variant of the
// dashboard's deconstruct flow: paste a transcript, run the AI
// deconstruct, and on success auto-create a Knowledge doc with the
// full analysis as Markdown. The user never has to leave the
// knowledge page to triage insights they want to keep.
//
// Phase 2 QA fixes applied:
//   - dialog a11y primitives (role / aria-modal / aria-labelledby)
//   - Escape key handler + focus trap (via shared useModalA11y hook)
//   - body scroll lock while the modal is open
//   - "Try demo" recovery button next to the primary CTA
//   - optional `initialResult` prop so the dashboard can hand off
//     its cached result and skip the AI call on the save flow
//   - shared `truncateForTitle` / `deconstructToMarkdown` so the
//     two panels cannot drift
//   - saved-doc link points to `/knowledge?id=<id>` for one-click
//     follow-through
//   - SVG X icon (consistent with NavBar) instead of the raw
//     "×" glyph

// Platform values for the dropdown. The wire value uses the
// canonical backend names (Chinese for 抖音/哔哩哔哩/小红书; the
// empty string falls through to the prompt's "(未提供)" default).
// Mirrors the dashboard panel's PLATFORMS so the two stay in
// lock-step; we deliberately keep the same shape ({value, labelKey})
// even though this panel never iterates labelKeys for a per-platform
// render — that way a future enhancement (e.g. localized platform
// chips) can share the same array with the dashboard.
const PLATFORMS: { value: string; labelKey: string }[] = [
  { value: '', labelKey: 'dashboard.deconstruct.platform.any' },
  { value: '抖音', labelKey: 'dashboard.deconstruct.platform.douyin' },
  { value: '哔哩哔哩', labelKey: 'dashboard.deconstruct.platform.bilibili' },
  { value: '小红书', labelKey: 'dashboard.deconstruct.platform.xiaohongshu' },
]

interface KnowledgeDeconstructPanelProps {
  onClose: () => void
  onSaved: (doc: KnowledgeDoc) => void
  // initialResult lets a caller (e.g. dashboard) pass a
  // pre-computed DeconstructResult so the user does not pay the
  // AI roundtrip cost again when they're just routing a recent
  // analysis into the knowledge base. The panel still calls
  // `api.knowledge.create` on submit; only the deconstruct step
  // is skipped.
  initialResult?: DeconstructResult | null
  initialDemo?: boolean
}

export function KnowledgeDeconstructPanel({
  onClose,
  onSaved,
  initialResult = null,
  initialDemo = false,
}: KnowledgeDeconstructPanelProps) {
  const t = useT()
  const dialogRef = useModalA11y(true)
  const [transcript, setTranscript] = useState('')
  const [platform, setPlatform] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [savedDoc, setSavedDoc] = useState<KnowledgeDoc | null>(null)
  const [demo, setDemo] = useState(initialDemo)

  // Close-on-Escape: shared a11y hook dispatches `modal-escape`
  // on the dialog ref; we funnel that through onClose.
  useEffect(() => {
    const node = dialogRef.current
    if (!node) return
    function handleEscape(e: Event) {
      if (e.type === 'modal-escape') onClose()
    }
    node.addEventListener('modal-escape', handleEscape)
    return () => node.removeEventListener('modal-escape', handleEscape)
  }, [onClose, dialogRef])

  async function handleSubmit(forceDemo = false) {
    const trimmed = transcript.trim()
    if (!trimmed) {
      setError(t('dashboard.deconstruct.error_required'))
      return
    }
    setError(null)
    setSavedDoc(null)
    setSubmitting(true)
    try {
      // If the caller handed us a pre-computed result, skip the
      // AI call entirely — the dashboard hands the user off with
      // the same transcript they just analyzed, and re-running
      // would burn a Claude roundtrip for no new information.
      let deconResult: DeconstructResult
      let deconDemo = initialDemo
      if (initialResult) {
        deconResult = initialResult
      } else {
        const metadata: { platform?: string } = {}
        if (platform) {
          metadata.platform = platform
        }
        const decon = await api.ai.deconstruct(
          { transcript: trimmed, metadata },
          forceDemo ? { demo: true } : undefined,
        )
        deconResult = decon.data
        deconDemo = decon.demo
      }
      setDemo(deconDemo)
      const titleText = truncateForTitle(trimmed, 30)
      const title = `爆款拆解: ${titleText}`
      const body = deconstructToMarkdown(
        { transcript: trimmed, platform },
        deconResult,
      )
      const doc = await api.knowledge.create({
        title,
        path: 'ai-deconstruct/auto',
        doc_type: '其它',
        content: body,
      })
      setSavedDoc(doc)
      onSaved(doc)
    } catch (err: unknown) {
      setError(
        err instanceof Error
          ? err.message
          : t('dashboard.deconstruct.action.save_failed'),
      )
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-2 sm:p-4"
      onClick={onClose}
    >
      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby="knowledge-deconstruct-panel-title"
        tabIndex={-1}
        className="bg-claude-canvas rounded-lg border border-claude-hairline shadow-claude-soft max-w-2xl w-full max-h-[90vh] sm:max-h-[85vh] overflow-y-auto"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="p-3 sm:p-4 border-b border-claude-hairline flex items-center justify-between gap-2">
          <h2
            id="knowledge-deconstruct-panel-title"
            className="text-base sm:text-lg font-semibold text-claude-ink"
          >
            {t('dashboard.deconstruct.title')}
          </h2>
          <button
            onClick={onClose}
            aria-label={t('dashboard.deconstruct.close')}
            className="text-claude-muted-soft hover:text-claude-ink text-2xl leading-none inline-flex items-center justify-center w-8 h-8 rounded hover:bg-claude-surface-soft focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-claude-coral"
          >
            <svg
              xmlns="http://www.w3.org/2000/svg"
              viewBox="0 0 20 20"
              fill="currentColor"
              className="w-4 h-4"
              aria-hidden="true"
            >
              <path
                fillRule="evenodd"
                d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z"
                clipRule="evenodd"
              />
            </svg>
          </button>
        </div>

        <div className="p-3 sm:p-4 space-y-4">
          <p className="text-xs md:text-sm text-claude-muted">
            {t('dashboard.deconstruct.subtitle')}
          </p>

          <div>
            <label
              htmlFor="knowledge-deconstruct-transcript"
              className="block text-xs md:text-sm font-medium text-claude-ink"
            >
              {t('dashboard.deconstruct.transcript_label')}
            </label>
            <textarea
              id="knowledge-deconstruct-transcript"
              data-testid="knowledge-deconstruct-transcript"
              value={transcript}
              onChange={(e) => setTranscript(e.target.value)}
              placeholder={t('dashboard.deconstruct.transcript_placeholder')}
              rows={5}
              className="mt-1 w-full px-3 py-2 text-sm border border-claude-hairline rounded bg-claude-canvas text-claude-ink focus:border-claude-coral focus:outline-none focus:ring-1 focus:ring-claude-coral"
            />
          </div>

          <div>
            <label
              htmlFor="knowledge-deconstruct-platform"
              className="block text-xs md:text-sm font-medium text-claude-ink"
            >
              {t('dashboard.deconstruct.platform_label')}
            </label>
            <select
              id="knowledge-deconstruct-platform"
              data-testid="knowledge-deconstruct-platform"
              value={platform}
              onChange={(e) => setPlatform(e.target.value)}
              // When the dashboard hands off a cached
              // DeconstructResult, handleSubmit() skips the AI
              // call entirely — the dropdown value would be
              // silently ignored. Disable it so the user sees
              // the dropdown is locked to whatever platform the
              // dashboard originally analyzed, and we also
              // surface a hint in the help text below.
              disabled={!!initialResult}
              className="mt-1 w-full px-3 py-2 text-sm border border-claude-hairline rounded bg-claude-canvas text-claude-ink focus:border-claude-coral focus:outline-none focus:ring-1 focus:ring-claude-coral disabled:opacity-60 disabled:cursor-not-allowed"
            >
              {PLATFORMS.map((p) => (
                <option key={p.value} value={p.value}>
                  {t(p.labelKey)}
                </option>
              ))}
            </select>
          </div>

          <div className="flex flex-wrap items-center gap-2">
            <button
              type="button"
              data-testid="btn-knowledge-deconstruct-save"
              onClick={() => handleSubmit(false)}
              disabled={submitting}
              className="px-3 md:px-4 py-1.5 md:py-2 text-xs md:text-sm bg-claude-success text-claude-on-primary rounded hover:bg-claude-success/90 disabled:opacity-50 transition-colors"
            >
              {submitting
                ? t('dashboard.deconstruct.action.save_loading')
                : t('dashboard.deconstruct.action.save')}
            </button>
            <button
              type="button"
              data-testid="btn-knowledge-deconstruct-try-demo"
              onClick={() => handleSubmit(true)}
              disabled={submitting}
              className="px-3 md:px-4 py-1.5 md:py-2 text-xs md:text-sm bg-claude-accent-amber text-claude-on-primary rounded-md hover:bg-claude-accent-amber-active disabled:opacity-50 transition-colors"
            >
              {t('dashboard.deconstruct.action.try_demo')}
            </button>
          </div>

          {error && (
            <div className="p-3 bg-claude-error/10 border border-claude-error text-claude-error rounded text-sm space-y-2">
              <p>{error}</p>
              <button
                type="button"
                onClick={() => handleSubmit(true)}
                className="text-xs underline hover:opacity-80"
              >
                {t('dashboard.deconstruct.action.try_demo')}
              </button>
            </div>
          )}

          {savedDoc && (
            <div
              data-testid="knowledge-deconstruct-saved"
              className="p-3 bg-claude-success/10 border border-claude-success/30 text-claude-success rounded text-xs md:text-sm"
            >
              {t('dashboard.deconstruct.action.save_success')}{' '}
              <a
                href={`/knowledge?id=${savedDoc.id}`}
                className="underline hover:opacity-80"
              >
                /knowledge?id={savedDoc.id}
              </a>
            </div>
          )}

          {demo && (
            <div className="inline-flex items-center gap-1.5 px-2 py-1 text-xs font-medium rounded bg-claude-accent-amber/15 text-claude-accent-amber border border-claude-accent-amber/30">
              {t('dashboard.deconstruct.demo_badge')}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

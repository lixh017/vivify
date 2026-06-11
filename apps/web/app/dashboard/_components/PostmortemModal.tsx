'use client'

import type { PostmortemStructured } from '@opc/shared/api'
import type { ContentItem } from '@opc/shared/types'
import { useT } from '@opc/shared/i18n-client'

// PostmortemModal — inline component for the AI postmortem report
// viewer. State machine: null (closed) | {loading} (in flight) |
// {result} (done) | {error} (failed). The modal stays mounted while
// the result is shown so the Close button can dismiss it cleanly.
export interface PostmortemModalState {
  loading: boolean
  report: string
  structured: PostmortemStructured | null
  error: string | null
  demo: boolean
}

export const INITIAL_MODAL: PostmortemModalState = {
  loading: false,
  report: '',
  structured: null,
  error: null,
  demo: false,
}

// PostmortemModal — displays the AI postmortem report. When the
// structured parse succeeded we show the four buckets as a clear
// summary; the raw report text is always available below for
// transparency. When parsing failed or the call errored, we show
// whatever the API returned.
interface PostmortemModalProps {
  item: ContentItem
  state: PostmortemModalState
  onClose: () => void
}

export function PostmortemModal({ item, state, onClose }: PostmortemModalProps) {
  const t = useT()
  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-2 sm:p-4"
      onClick={onClose}
    >
      <div
        className="bg-claude-canvas rounded-lg border border-claude-hairline shadow-claude-soft max-w-2xl w-full max-h-[90vh] sm:max-h-[85vh] overflow-y-auto"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="p-3 sm:p-4 border-b border-claude-hairline flex items-center justify-between gap-2">
          <h2 className="text-base sm:text-lg font-semibold text-claude-ink">
            {t('dashboard.postmortem')} #{item.id}
            <span className="ml-2 text-xs text-claude-muted">({item.platform})</span>
          </h2>
          <button
            onClick={onClose}
            aria-label={t('dashboard.aria.close')}
            className="text-claude-muted-soft hover:text-claude-ink text-2xl leading-none"
          >
            ×
          </button>
        </div>

        <div className="p-3 sm:p-4 space-y-4">
          {state.demo && !state.loading && !state.error && (
            <div
              data-testid="demo-badge-postmortem"
              className="inline-flex items-center gap-1.5 px-2 py-1 text-xs font-medium rounded bg-claude-accent-amber/15 text-claude-accent-amber border border-claude-accent-amber/30"
            >
              {t('dashboard.postmortem_demo')}
            </div>
          )}
          {state.loading && (
            <div className="text-sm text-claude-body">
              {t('dashboard.postmortem_loading_view')}
            </div>
          )}

          {state.error && (
            <div className="p-3 bg-claude-error/10 border border-claude-error text-claude-error rounded text-sm">
              {state.error}
            </div>
          )}

          {!state.loading && !state.error && state.structured && (
            <PostmortemStructuredView structured={state.structured} />
          )}

          {!state.loading && !state.error && state.report && (
            <details className="text-sm">
              <summary className="cursor-pointer text-claude-body hover:text-claude-ink">
                {t('dashboard.postmortem_raw')}
              </summary>
              <pre className="mt-2 p-3 bg-claude-surface-card rounded whitespace-pre-wrap break-words text-xs text-claude-ink">
                {state.report}
              </pre>
            </details>
          )}
        </div>

        <div className="p-3 sm:p-4 border-t border-claude-hairline flex justify-end">
          <button
            onClick={onClose}
            className="w-full sm:w-auto px-4 py-2 text-sm bg-claude-surface-card text-claude-ink rounded hover:bg-claude-surface-soft border border-claude-hairline"
          >
            {t('dashboard.aria.close')}
          </button>
        </div>
      </div>
    </div>
  )
}

// PostmortemStructuredView renders the four parsed buckets. Each
// section is hidden when empty so a partial parse (e.g. Claude
// returned only success_factors) still renders cleanly.
interface PostmortemStructuredViewProps {
  structured: PostmortemStructured
}

function PostmortemStructuredView({ structured }: PostmortemStructuredViewProps) {
  const t = useT()
  return (
    <div className="space-y-3">
      <BucketSection
        title={t('dashboard.bucket.success')}
        items={structured.success_factors}
        accent="bg-claude-success/10 text-claude-success"
      />
      <BucketSection
        title={t('dashboard.bucket.patterns')}
        items={structured.reusable_patterns}
        accent="bg-claude-accent-teal/10 text-claude-accent-teal"
      />
      <BucketSection
        title={t('dashboard.bucket.insights')}
        items={structured.insights}
        accent="bg-claude-accent-amber/10 text-claude-accent-amber"
      />
      <BucketSection
        title={t('dashboard.bucket.suggestions')}
        items={structured.suggestions}
        accent="bg-claude-coral/10 text-claude-coral"
      />
    </div>
  )
}

interface BucketSectionProps {
  title: string
  items: string[]
  accent: string
}

function BucketSection({ title, items, accent }: BucketSectionProps) {
  if (!items || items.length === 0) return null
  return (
    <div>
      <h3 className={`inline-block px-2 py-0.5 rounded text-xs font-semibold ${accent}`}>
        {title}
      </h3>
      <ul className="mt-2 space-y-1 text-sm text-claude-ink list-disc list-inside">
        {items.map((it, i) => (
          <li key={i}>{it}</li>
        ))}
      </ul>
    </div>
  )
}

'use client'

import type { GeneratedTopic } from '@opc/shared/api'
import { useT } from '@opc/shared/i18n-client'

// TopicsModal — inline component for the AI-generated topics viewer.
// State machine: null (closed) | {loading} (in flight) | {topics} (done)
// | {error} (failed). The modal stays mounted while topics are shown
// so the Close button can dismiss it cleanly.
export interface TopicsModalState {
  loading: boolean
  topics: GeneratedTopic[]
  error: string | null
  demo: boolean
}

export const INITIAL_TOPICS_MODAL: TopicsModalState = {
  loading: false,
  topics: [],
  error: null,
  demo: false,
}

// TopicsModal — displays the AI-generated topic ideas. Each topic
// shows its title, angle, hook, and expected performance so the
// operator can pick one to convert into a ContentItem (the
// "dashboard 看历史" half of G1 already exists via loadItems).
//
// The header carries a `seed` text input + a Regenerate button.
// Power users can edit the seed and click Regenerate to re-fire
// the API with a different concept. The default seed is owned
// by the DashboardPage so it persists across modal open/close.
interface TopicsModalProps {
  state: TopicsModalState
  seed: string
  onSeedChange: (next: string) => void
  onRegenerate: () => void
  onClose: () => void
}

export function TopicsModal({ state, seed, onSeedChange, onRegenerate, onClose }: TopicsModalProps) {
  const t = useT()
  return (
    <div
      data-testid="topics-modal"
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-2 sm:p-4"
      onClick={onClose}
    >
      <div
        className="bg-claude-canvas rounded-lg border border-claude-hairline shadow-claude-soft max-w-2xl w-full max-h-[90vh] sm:max-h-[85vh] overflow-y-auto"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="p-3 sm:p-4 border-b border-claude-hairline space-y-2">
          <div className="flex items-start justify-between gap-2">
            <div>
              <h2 className="text-base sm:text-lg font-semibold text-claude-ink">
                {t('dashboard.generate_topics.title')}
              </h2>
              <p className="text-xs text-claude-muted mt-0.5">
                {t('dashboard.generate_topics.subtitle')}
              </p>
            </div>
            <button
              onClick={onClose}
              aria-label={t('dashboard.aria.close')}
              className="text-claude-muted-soft hover:text-claude-ink text-2xl leading-none"
            >
              ×
            </button>
          </div>
          <div className="flex items-center gap-2">
            <label className="text-xs text-claude-muted shrink-0">
              {t('dashboard.generate_topics.seed_label')}
            </label>
            <input
              data-testid="topics-seed-input"
              type="text"
              value={seed}
              onChange={(e) => onSeedChange(e.target.value)}
              placeholder={t('dashboard.generate_topics.seed_placeholder')}
              className="flex-1 min-w-0 px-2 py-1 text-sm border border-claude-hairline rounded bg-claude-canvas text-claude-ink focus:border-claude-coral focus:outline-none focus:ring-1 focus:ring-claude-coral"
            />
            <button
              type="button"
              data-testid="topics-regenerate"
              onClick={onRegenerate}
              disabled={state.loading || !seed.trim()}
              className="shrink-0 px-3 py-1 text-xs bg-claude-coral text-claude-on-primary rounded hover:bg-claude-coral-active disabled:opacity-50 transition-colors"
            >
              {state.loading
                ? t('dashboard.postmortem_loading')
                : t('dashboard.generate_topics.regenerate')}
            </button>
          </div>
        </div>

        <div className="p-3 sm:p-4 space-y-3">
          {state.demo && !state.loading && !state.error && (
            <div
              data-testid="demo-badge-topics"
              className="inline-flex items-center gap-1.5 px-2 py-1 text-xs font-medium rounded bg-claude-accent-amber/15 text-claude-accent-amber border border-claude-accent-amber/30"
            >
              {t('dashboard.generate_topics.demo_badge')}
            </div>
          )}

          {state.loading && (
            <div
              data-testid="topics-loading"
              className="text-sm text-claude-body"
            >
              {t('dashboard.postmortem_loading_view')}
            </div>
          )}

          {state.error && (
            <div
              data-testid="topics-error"
              className="p-3 bg-claude-error/10 border border-claude-error text-claude-error rounded text-sm"
            >
              {state.error}
            </div>
          )}

          {!state.loading && !state.error && state.topics.length === 0 && (
            <div
              data-testid="topics-empty"
              className="p-4 bg-claude-surface-card rounded-lg border border-claude-hairline text-sm text-claude-muted text-center"
            >
              {t('dashboard.generate_topics.empty')}
            </div>
          )}

          {!state.loading &&
            !state.error &&
            state.topics.map((topic, i) => (
              <div
                key={i}
                data-testid={`topic-item-${i}`}
                className="p-3 bg-claude-surface-card rounded-lg border border-claude-hairline"
              >
                <h3 className="font-serif text-base text-claude-ink font-semibold">
                  {topic.title}
                </h3>
                <div className="mt-2 space-y-1 text-xs text-claude-body">
                  <p>
                    <span className="font-medium text-claude-muted">
                      {t('dashboard.generate_topics.angle')}:
                    </span>{' '}
                    {topic.angle}
                  </p>
                  <p>
                    <span className="font-medium text-claude-muted">
                      {t('dashboard.generate_topics.hook')}:
                    </span>{' '}
                    {topic.hook}
                  </p>
                  <p>
                    <span className="font-medium text-claude-muted">
                      {t('dashboard.generate_topics.expected')}:
                    </span>{' '}
                    {topic.expected_performance}
                  </p>
                </div>
              </div>
            ))}
        </div>

        <div className="p-3 sm:p-4 border-t border-claude-hairline flex justify-end">
          <button
            onClick={onClose}
            className="w-full sm:w-auto px-4 py-2 text-sm bg-claude-surface-card text-claude-ink rounded hover:bg-claude-surface-soft border border-claude-hairline"
          >
            {t('dashboard.generate_topics.close')}
          </button>
        </div>
      </div>
    </div>
  )
}

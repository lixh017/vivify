'use client'

import { useState, useCallback, useRef, useEffect } from 'react'
import ScrollToFeaturesLink from './ScrollToFeaturesLink'
import { useT } from '@/lib/i18n-client'
import { api } from '@/lib/api'
import type { PipelineResult, QualitySuggestion } from '@/lib/api'
import { useModalA11y } from '@/lib/use-modal'

interface FeatureCard {
  href: string
  icon: string
  titleKey:
    | 'home.features.card.topics.title'
    | 'home.features.card.scripts.title'
    | 'home.features.card.calendar.title'
    | 'home.features.card.dashboard.title'
    | 'home.features.card.knowledge.title'
  descKey:
    | 'home.features.card.topics.desc'
    | 'home.features.card.scripts.desc'
    | 'home.features.card.calendar.desc'
    | 'home.features.card.dashboard.desc'
    | 'home.features.card.knowledge.desc'
}

const FEATURES: FeatureCard[] = [
  {
    href: '/topics',
    icon: '📋',
    titleKey: 'home.features.card.topics.title',
    descKey: 'home.features.card.topics.desc',
  },
  {
    href: '/scripts',
    icon: '📝',
    titleKey: 'home.features.card.scripts.title',
    descKey: 'home.features.card.scripts.desc',
  },
  {
    href: '/calendar',
    icon: '📅',
    titleKey: 'home.features.card.calendar.title',
    descKey: 'home.features.card.calendar.desc',
  },
  {
    href: '/dashboard',
    icon: '📊',
    titleKey: 'home.features.card.dashboard.title',
    descKey: 'home.features.card.dashboard.desc',
  },
  {
    href: '/knowledge',
    icon: '📚',
    titleKey: 'home.features.card.knowledge.title',
    descKey: 'home.features.card.knowledge.desc',
  },
]

// PipelinePanel is the modal launched from the home page's
// "🚀 One-shot Pipeline (demo)" button. It runs the full
// topics → script → score → 3-platform adapt chain in a single
// /ai/pipeline call (demo mode is forced via `demo: true` so
// the operator can show the feature in a sales context without
// burning tokens). The modal renders each step in its own
// section so the operator can see the chain in one place.
//
// a11y: useModalA11y wires up the body scroll lock, focus trap,
// Escape handling, and focus restoration that the rest of the
// modals in the app share. The trigger button also moves focus
// back to itself on close.
function PipelinePanel({ onClose }: { onClose: () => void }) {
  const t = useT()
  const dialogRef = useModalA11y(true)
  const [phase, setPhase] = useState<'idle' | 'loading' | 'result' | 'error'>(
    'idle',
  )
  const [result, setResult] = useState<PipelineResult | null>(null)
  const [demo, setDemo] = useState<boolean>(false)
  const [errorMsg, setErrorMsg] = useState<string>('')

  // Run the pipeline. We force demo mode unconditionally here
  // because the home-page CTA is the "look at the whole chain
  // in one click" path — the live-mode experience lives on
  // /topics + /scripts + /dashboard, where the operator can
  // poke at individual steps.
  const handleRun = useCallback(async () => {
    setPhase('loading')
    setErrorMsg('')
    try {
      const r = await api.ai.pipeline(
        {
          seed: '雨夜',
          platform: '抖音',
          include_knowledge: true,
        },
        { demo: true },
      )
      setResult(r.data)
      setDemo(r.demo)
      setPhase('result')
    } catch (err) {
      setErrorMsg(err instanceof Error ? err.message : String(err))
      setPhase('error')
    }
  }, [])

  // Wire the useModalA11y Escape dispatch back to onClose.
  useEffect(() => {
    const dlg = dialogRef.current
    if (!dlg) return
    const handler = () => onClose()
    dlg.addEventListener('modal-escape', handler)
    return () => dlg.removeEventListener('modal-escape', handler)
  }, [dialogRef, onClose])

  return (
    <div
      className="fixed inset-0 bg-black/40 z-50 flex items-center justify-center p-4"
      onClick={onClose}
      data-testid="pipeline-modal-backdrop"
    >
      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby="pipeline-modal-title"
        tabIndex={-1}
        onClick={(e) => e.stopPropagation()}
        className="bg-claude-surface-card rounded-lg shadow-xl max-w-3xl w-full max-h-[90vh] overflow-y-auto border border-claude-hairline"
      >
        {/* Header */}
        <div className="sticky top-0 bg-claude-surface-card border-b border-claude-hairline px-6 py-4 flex items-center justify-between">
          <div>
            <h2
              id="pipeline-modal-title"
              className="font-serif text-xl text-claude-ink"
            >
              {t('home.pipeline.title')}
            </h2>
            <p className="text-sm text-claude-body mt-1">
              {t('home.pipeline.subhead')}
            </p>
          </div>
          <button
            type="button"
            onClick={onClose}
            data-testid="pipeline-modal-close"
            className="text-claude-muted hover:text-claude-ink focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-claude-coral rounded-sm p-1"
            aria-label={t('home.pipeline.close')}
          >
            ✕
          </button>
        </div>

        {/* Body */}
        <div className="px-6 py-6 space-y-6">
          {/* Idle / Loading / Error: show the run button */}
          {phase !== 'result' && (
            <div className="text-center py-8">
              {phase === 'idle' && (
                <button
                  type="button"
                  onClick={handleRun}
                  data-testid="pipeline-run"
                  className="inline-flex items-center justify-center bg-claude-coral text-claude-on-primary px-5 py-2.5 rounded-md text-sm font-medium hover:bg-claude-coral-active focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-claude-ink focus-visible:ring-offset-2 active:translate-y-px transition-colors"
                >
                  {t('home.pipeline.run')}
                </button>
              )}
              {phase === 'loading' && (
                <div className="text-claude-muted">
                  <div className="inline-block w-4 h-4 border-2 border-claude-coral border-t-transparent rounded-full animate-spin mr-2" />
                  {t('home.pipeline.running')}
                </div>
              )}
              {phase === 'error' && (
                <div className="space-y-3">
                  <p className="text-claude-error text-sm">
                    {t('home.pipeline.error')}
                  </p>
                  <p className="text-claude-muted text-xs">{errorMsg}</p>
                  <button
                    type="button"
                    onClick={handleRun}
                    className="text-sm text-claude-coral hover:underline"
                  >
                    {t('home.pipeline.run')}
                  </button>
                </div>
              )}
            </div>
          )}

          {/* Result: show the 4 sections + knowledge_used line */}
          {phase === 'result' && result && (
            <div className="space-y-6">
              {demo && (
                <div className="text-xs text-claude-accent-amber bg-claude-surface-soft rounded-md px-3 py-2 border border-claude-hairline">
                  {t('demo.badge')}
                </div>
              )}
              {result.knowledge_used && result.knowledge_used.length > 0 && (
                <p className="text-xs text-claude-muted">
                  {t('home.pipeline.knowledge', { n: result.knowledge_used.length })}
                </p>
              )}

              {/* ① Topics */}
              {result.topics && result.topics.length > 0 && (
                <PipelineSection title={t('home.pipeline.section.topics')}>
                  <ul className="space-y-2">
                    {result.topics.map((t, i) => (
                      <li
                        key={i}
                        className="bg-claude-surface-soft rounded-md p-3 border border-claude-hairline"
                      >
                        <p className="font-medium text-claude-ink">{t.title}</p>
                        <p className="text-sm text-claude-body mt-1">{t.angle}</p>
                      </li>
                    ))}
                  </ul>
                </PipelineSection>
              )}

              {/* ② Script */}
              {result.script && (
                <PipelineSection title={t('home.pipeline.section.script')}>
                  <div className="bg-claude-surface-soft rounded-md p-4 border border-claude-hairline">
                    <p className="text-xs text-claude-muted mb-2">
                      {result.script.title}
                    </p>
                    <pre className="font-sans text-sm text-claude-ink whitespace-pre-wrap font-sans">
                      {result.script.content}
                    </pre>
                  </div>
                </PipelineSection>
              )}

              {/* ③ Score */}
              {result.score && (
                <PipelineSection title={t('home.pipeline.section.score')}>
                  <div className="bg-claude-surface-soft rounded-md p-4 border border-claude-hairline">
                    <div className="grid grid-cols-4 gap-3 mb-3">
                      <ScoreCell label="综合" value={result.score.overall_score} />
                      <ScoreCell label="钩子" value={result.score.hook_strength} />
                      <ScoreCell label="结构" value={result.score.structure} />
                      <ScoreCell label="平台" value={result.score.platform_fit} />
                    </div>
                    {result.score.suggestions && result.score.suggestions.length > 0 && (
                      <ul className="text-xs text-claude-body space-y-1">
                        {result.score.suggestions.map(
                          (s: QualitySuggestion, i: number) => (
                            <li key={i}>
                              <span className="text-claude-muted">[{s.severity}]</span> {s.problem}
                            </li>
                          ),
                        )}
                      </ul>
                    )}
                  </div>
                </PipelineSection>
              )}

              {/* ④ Adapt */}
              {result.adaptations && (
                <PipelineSection title={t('home.pipeline.section.adapt')}>
                  <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
                    <AdaptCard
                      title={result.adaptations.adaptations.抖音.title}
                      tags={result.adaptations.adaptations.抖音.hashtags}
                      body={result.adaptations.adaptations.抖音.description}
                      label="抖音"
                    />
                    <AdaptCard
                      title={result.adaptations.adaptations.哔哩哔哩.title}
                      tags={result.adaptations.adaptations.哔哩哔哩.tags}
                      body={result.adaptations.adaptations.哔哩哔哩.description}
                      label="哔哩哔哩"
                    />
                    <AdaptCard
                      title={result.adaptations.adaptations.小红书.title}
                      tags={result.adaptations.adaptations.小红书.tags}
                      body={result.adaptations.adaptations.小红书.body}
                      label="小红书"
                    />
                  </div>
                </PipelineSection>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

// PipelineSection is the small framed block the modal uses for
// each of the 4 chain steps. Kept inline here so the file
// reads top-down.
function PipelineSection({
  title,
  children,
}: {
  title: string
  children: React.ReactNode
}) {
  return (
    <section>
      <h3 className="font-serif text-sm text-claude-muted mb-2">{title}</h3>
      {children}
    </section>
  )
}

function ScoreCell({ label, value }: { label: string; value: number }) {
  return (
    <div className="text-center">
      <div className="text-2xl font-serif text-claude-coral">{value}</div>
      <div className="text-xs text-claude-muted mt-1">{label}</div>
    </div>
  )
}

function AdaptCard({
  title,
  tags,
  body,
  label,
}: {
  title: string
  tags: string[]
  body: string
  label: string
}) {
  return (
    <div className="bg-claude-surface-soft rounded-md p-3 border border-claude-hairline">
      <div className="text-xs text-claude-muted mb-1">{label}</div>
      <p className="font-medium text-claude-ink text-sm">{title}</p>
      <p className="text-xs text-claude-body mt-2 whitespace-pre-wrap">{body}</p>
      {tags && tags.length > 0 && (
        <div className="flex flex-wrap gap-1 mt-2">
          {tags.map((t, i) => (
            <span
              key={i}
              className="text-[10px] text-claude-muted bg-claude-canvas px-2 py-0.5 rounded"
            >
              {t}
            </span>
          ))}
        </div>
      )}
    </div>
  )
}

export default function Home() {
  const t = useT()
  const [showPipeline, setShowPipeline] = useState(false)

  return (
    <>
      {/* Hero */}
      <section className="max-w-4xl mx-auto px-6 py-section">
        <p className="claude-eyebrow text-claude-muted">
          {t('home.eyebrow')}
        </p>
        <h1 className="mt-4 text-claude-ink leading-tight">
          {t('home.title')}
        </h1>
        <p className="font-sans text-lg text-claude-body mt-6 max-w-2xl">
          {t('home.subhead')}
        </p>
        <div className="mt-10 flex gap-3 flex-wrap">
          <a
            href="/topics"
            className="inline-flex items-center justify-center bg-claude-coral text-claude-on-primary px-5 py-2.5 rounded-md text-sm font-medium hover:bg-claude-coral-active focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-claude-ink focus-visible:ring-offset-2 active:translate-y-px transition-colors"
          >
            {t('home.cta.primary')}
          </a>
          <ScrollToFeaturesLink className="inline-flex items-center justify-center border border-claude-hairline text-claude-ink px-5 py-2.5 rounded-md text-sm font-medium hover:bg-claude-surface-card focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-claude-coral focus-visible:ring-offset-2 active:translate-y-px transition-colors">
            {t('home.cta.secondary')}
          </ScrollToFeaturesLink>
          {/* "Try Demo" — drives the user straight into a no-signup
              AI experience. Lands on /topics?demo=true which forces
              every /ai/* call through canned data and surfaces the
              demo badge. Styled as a tertiary surface chip so it
              reads as opt-in rather than competing with the primary
              entry. */}
          <a
            href="/topics?demo=true"
            data-testid="cta-try-demo"
            className="inline-flex items-center justify-center bg-claude-surface-card text-claude-ink border border-claude-hairline px-5 py-2.5 rounded-md text-sm font-medium hover:bg-claude-surface-soft focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-claude-accent-amber focus-visible:ring-offset-2 active:translate-y-px transition-colors"
          >
            {t('home.cta.demo')}
          </a>
          {/* "One-shot Pipeline" — opens a modal that runs the
              full topics → script → score → 3-platform adapt
              chain in a single demo call. The "show me the whole
              loop" entry point: best as a button (modal) rather
              than a navigation link, since it does not leave the
              landing page. */}
          <button
            type="button"
            onClick={() => setShowPipeline(true)}
            data-testid="cta-pipeline"
            className="inline-flex items-center justify-center bg-claude-surface-card text-claude-ink border border-claude-hairline px-5 py-2.5 rounded-md text-sm font-medium hover:bg-claude-surface-soft focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-claude-coral focus-visible:ring-offset-2 active:translate-y-px transition-colors"
          >
            {t('home.cta.pipeline')}
          </button>
        </div>
      </section>

      {/* 5 feature cards */}
      <section id="features" className="max-w-6xl mx-auto px-6 py-section">
        <p className="claude-eyebrow text-claude-muted">
          {t('home.features.eyebrow')}
        </p>
        <h2 className="mt-4 text-claude-ink">{t('home.features.title')}</h2>
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6 mt-12">
          {FEATURES.map((card) => (
            <article
              key={card.href}
              className="bg-claude-surface-card rounded-lg p-8 border border-claude-hairline"
            >
              <div aria-hidden="true" className="text-3xl">
                {card.icon}
              </div>
              <h3 className="font-serif text-xl text-claude-ink mt-4">
                {t(card.titleKey)}
              </h3>
              <p className="font-sans text-sm text-claude-body mt-2">
                {t(card.descKey)}
              </p>
              <a
                href={card.href}
                className="text-claude-coral text-sm font-medium mt-4 inline-block hover:text-claude-coral-active focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-claude-coral focus-visible:ring-offset-2 rounded-sm transition-colors"
              >
                {t('home.features.card.cta')}
              </a>
            </article>
          ))}
        </div>
      </section>

      {/* One-shot pipeline modal */}
      {showPipeline && <PipelinePanel onClose={() => setShowPipeline(false)} />}
    </>
  )
}

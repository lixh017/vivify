import ScrollToFeaturesLink from './ScrollToFeaturesLink'
import { t } from '@/lib/i18n'

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

export default function Home() {
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
    </>
  )
}

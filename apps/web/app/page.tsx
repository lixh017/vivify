import ScrollToFeaturesLink from './ScrollToFeaturesLink'

interface FeatureCard {
  href: string
  icon: string
  title: string
  description: string
}

const FEATURES: FeatureCard[] = [
  {
    href: '/topics',
    icon: '📋',
    title: '选题',
    description: 'Kanban 视图,状态切换,AI 智能生成',
  },
  {
    href: '/scripts',
    icon: '📝',
    title: '脚本',
    description: 'Markdown 编辑,AI 拟人化改写',
  },
  {
    href: '/calendar',
    icon: '📅',
    title: '日历',
    description: '发布排期,内联日期编辑',
  },
  {
    href: '/dashboard',
    icon: '📊',
    title: '表现',
    description: '数据汇总,爆款 AI 复盘',
  },
  {
    href: '/knowledge',
    icon: '📚',
    title: '知识库',
    description: 'IP 风格指南,SOP 沉淀,FTS5 搜索',
  },
]

export default function Home() {
  return (
    <>
      {/* Hero */}
      <section className="max-w-4xl mx-auto px-6 py-section">
        <p className="claude-eyebrow text-claude-muted">
          Phase 1 · 熊猫 IP · Content Studio
        </p>
        <h1 className="mt-4 text-claude-ink leading-tight">
          A quiet room for your short video work.
        </h1>
        <p className="font-sans text-lg text-claude-body mt-6 max-w-2xl">
          选题、脚本、素材、知识库 — 一处安放,安静创作.
        </p>
        <div className="mt-10 flex gap-3">
          <a
            href="/topics"
            className="inline-flex items-center justify-center bg-claude-coral text-claude-on-primary px-5 py-2.5 rounded-md text-sm font-medium hover:bg-claude-coral-active focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-claude-ink focus-visible:ring-offset-2 active:translate-y-px transition-colors"
          >
            进入工作室
          </a>
          <ScrollToFeaturesLink className="inline-flex items-center justify-center border border-claude-hairline text-claude-ink px-5 py-2.5 rounded-md text-sm font-medium hover:bg-claude-surface-card focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-claude-coral focus-visible:ring-offset-2 active:translate-y-px transition-colors">
            了解更多
          </ScrollToFeaturesLink>
        </div>
      </section>

      {/* 5 feature cards */}
      <section id="features" className="max-w-6xl mx-auto px-6 py-section">
        <p className="claude-eyebrow text-claude-muted">5 块能力</p>
        <h2 className="mt-4 text-claude-ink">Content Studio,end-to-end.</h2>
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
                {card.title}
              </h3>
              <p className="font-sans text-sm text-claude-body mt-2">
                {card.description}
              </p>
              <a
                href={card.href}
                className="text-claude-coral text-sm font-medium mt-4 inline-block hover:text-claude-coral-active focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-claude-coral focus-visible:ring-offset-2 rounded-sm transition-colors"
              >
                去看看 →
              </a>
            </article>
          ))}
        </div>
      </section>
    </>
  )
}

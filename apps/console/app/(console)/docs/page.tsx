import Link from 'next/link'
import { TopBar } from '@/components/TopBar'
import { SKILLS, skillsByCategory, type SkillMeta } from '@/lib/skills-registry'

// /docs is the operator-facing reference for every skill the
// OPC backend exposes. We render three category sections (ai /
// media / admin) and a card per skill. Each card links to the
// dynamic detail page at /docs/skills/[id].
//
// On top of the per-skill docs we surface a "通用文档" section
// with three hand-authored markdown pages (auth / rate-limits /
// error-codes). These are not in the registry — they describe
// cross-cutting concerns that apply to every skill.
//
// Categories are hard-coded to control section ordering — a
// future sort-by-name refactor can still iterate over
// skillsByCategory() for content; the order, however, is
// authored here on purpose (ops first, then AI, then media, then admin).

const CATEGORY_ORDER: { key: SkillMeta['category']; title: string; blurb: string }[] = [
  {
    key: 'ai',
    title: 'AI 内容生成',
    blurb: '选题、脚本、评分、平台适配、爆款拆解 — 一组 8 个 skill, 可以单独调, 也可以用 pipeline / batch 串起来。',
  },
  {
    key: 'media',
    title: '媒体生成',
    blurb: '封面图、未来的口播音频 / 视频 — 这部分接的是 Kling / 即梦 / 其它视觉 provider, 单价偏高。',
  },
  {
    key: 'admin',
    title: '运营管理',
    blurb: '凭据、调用日志、未来的成员 / 计费 — 内部端点, 0 成本, 通常不限流。',
  },
]

// Hand-authored markdown docs that describe cross-cutting
// concerns. Order in this array = order on the page.
const OPS_DOCS: { slug: string; title: string; blurb: string }[] = [
  {
    slug: 'auth',
    title: '鉴权',
    blurb: 'opc_session cookie 体系、登录端点、续期策略、跨域 cookie、MCP 客户端接入。',
  },
  {
    slug: 'rate-limits',
    title: '限流',
    blurb: 'per-IP + per-user 双层限流、各 skill 的具体配额、429 响应与申请提额。',
  },
  {
    slug: 'error-codes',
    title: '错误码',
    blurb: '4xx / 5xx 全量定义、统一错误响应 envelope、上游供应商错误映射。',
  },
]

export default function DocsPage() {
  return (
    <>
      <TopBar title="接入文档" />
      <div className="p-6 space-y-8 max-w-5xl">
        <header className="space-y-2">
          <h2 className="text-lg font-medium text-claude-ink">Skill 接入文档</h2>
          <p className="text-sm text-claude-muted leading-relaxed max-w-2xl">
            这里覆盖 OPC 后端暴露的 {SKILLS.length} 个 skill 的 schema、调用示例、计费与限流。
            任何对 skill 的增删改都在 <code className="font-mono text-xs px-1 py-0.5 bg-claude-surface rounded">apps/console/lib/skills-registry.ts</code> 一处维护,
            本页和沙盒页面会自动跟上。
          </p>
        </header>

        <section aria-labelledby="ops-docs-heading" className="space-y-3">
          <div>
            <h3 id="ops-docs-heading" className="text-base font-medium text-claude-ink">
              通用文档
              <span className="ml-2 text-xs text-claude-muted font-normal">
                {OPS_DOCS.length} 篇
              </span>
            </h3>
            <p className="text-sm text-claude-muted mt-1">
              鉴权、限流、错误码 — 跨 skill 的共用约定, 接入前必读。
            </p>
          </div>
          <ul className="grid grid-cols-1 md:grid-cols-3 gap-3">
            {OPS_DOCS.map((doc) => (
              <li key={doc.slug}>
                <Link
                  href={`/docs/operations/${doc.slug}`}
                  className="block bg-white border border-claude-hairline rounded-lg p-4 shadow-claude-soft hover:border-claude-coral/40 hover:shadow-claude-medium transition-colors h-full"
                >
                  <h4 className="text-sm font-medium text-claude-ink">{doc.title}</h4>
                  <p className="text-xs text-claude-muted mt-1.5 line-clamp-3 leading-relaxed">
                    {doc.blurb}
                  </p>
                  <div className="mt-3 text-xs text-claude-coral">阅读文档 →</div>
                </Link>
              </li>
            ))}
          </ul>
        </section>

        {CATEGORY_ORDER.map((cat) => {
          const skills = skillsByCategory(cat.key)
          if (skills.length === 0) return null
          return (
            <section key={cat.key} aria-labelledby={`cat-${cat.key}`} className="space-y-3">
              <div>
                <h3
                  id={`cat-${cat.key}`}
                  className="text-base font-medium text-claude-ink"
                >
                  {cat.title}
                  <span className="ml-2 text-xs text-claude-muted font-normal">
                    {skills.length} 个 skill
                  </span>
                </h3>
                <p className="text-sm text-claude-muted mt-1">{cat.blurb}</p>
              </div>
              <ul className="grid grid-cols-1 md:grid-cols-2 gap-3">
                {skills.map((skill) => (
                  <li key={skill.id}>
                    <SkillCard skill={skill} />
                  </li>
                ))}
              </ul>
            </section>
          )
        })}

        <footer className="text-xs text-claude-muted pt-2 border-t border-claude-hairline">
          需要在浏览器里跑一次? 进 <Link href="/docs/sandbox" className="text-claude-coral hover:underline">沙盒</Link> 调一次真实接口。
        </footer>
      </div>
    </>
  )
}

interface SkillCardProps {
  skill: SkillMeta
}

function SkillCard({ skill }: SkillCardProps) {
  return (
    <Link
      href={`/docs/skills/${skill.id}`}
      className="block bg-white border border-claude-hairline rounded-lg p-4 shadow-claude-soft hover:border-claude-coral/40 hover:shadow-claude-medium transition-colors"
    >
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <h4 className="text-sm font-medium text-claude-ink truncate">{skill.name}</h4>
          <p className="text-xs text-claude-muted mt-1 line-clamp-2 leading-relaxed">
            {skill.description}
          </p>
        </div>
        <span
          className={
            'shrink-0 text-[10px] font-mono uppercase tracking-eyebrow px-1.5 py-0.5 rounded ' +
            methodColor(skill.method)
          }
          title={`HTTP ${skill.method}`}
        >
          {skill.method}
        </span>
      </div>
      <dl className="mt-3 grid grid-cols-2 gap-x-3 gap-y-1.5 text-xs">
        <div className="flex justify-between gap-2">
          <dt className="text-claude-muted">计费</dt>
          <dd className="text-claude-ink text-right truncate" title={skill.costCentsHint}>
            {skill.costCentsHint}
          </dd>
        </div>
        <div className="flex justify-between gap-2">
          <dt className="text-claude-muted">限流</dt>
          <dd className="text-claude-ink text-right">{skill.rateLimit}</dd>
        </div>
      </dl>
      <div className="mt-3 text-xs text-claude-coral">查看详情 →</div>
    </Link>
  )
}

function methodColor(method: SkillMeta['method']): string {
  switch (method) {
    case 'GET':
      return 'bg-emerald-50 text-emerald-700'
    case 'POST':
      return 'bg-claude-coral/10 text-claude-coral'
    case 'PUT':
      return 'bg-amber-50 text-amber-700'
    case 'DELETE':
      return 'bg-rose-50 text-rose-700'
    default:
      return 'bg-claude-surface text-claude-muted'
  }
}

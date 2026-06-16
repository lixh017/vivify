import Link from 'next/link'
import { notFound } from 'next/navigation'
import { TopBar } from '@/components/TopBar'
import { SKILLS, getSkill, type ParamSchema, type SkillMeta } from '@/lib/skills-registry'

// /docs/skills/[id] is the per-skill reference page. It shows
// the full request schema, a code-level example, and a
// "Run in sandbox" button that pre-selects this skill on the
// sandbox page via ?skill=<id>.
//
// Unknown id → notFound() renders Next's 404 + a "back to /docs"
// link is added in error.tsx / a custom not-found page is
// unnecessary because the parent /docs already lists every
// valid id — a typo is the only way to land here.

// SSG: pre-render every known skill id at build time. New skills
// added to the registry are picked up on the next build.
export function generateStaticParams(): { id: string }[] {
  return SKILLS.map((s) => ({ id: s.id }))
}

interface SkillDetailPageProps {
  params: { id: string }
}

export default function SkillDetailPage({ params }: SkillDetailPageProps) {
  const skill = getSkill(params.id)
  if (!skill) {
    notFound()
  }

  return (
    <>
      <TopBar title={skill.name} />
      <div className="p-6 space-y-6 max-w-4xl">
        <nav className="text-xs text-claude-muted">
          <Link href="/docs" className="hover:text-claude-ink">接入文档</Link>
          <span className="mx-1.5">/</span>
          <span className="text-claude-ink">{skill.name}</span>
        </nav>

        <header className="space-y-2">
          <div className="flex items-center gap-2 flex-wrap">
            <h2 className="text-lg font-medium text-claude-ink">{skill.name}</h2>
            <CategoryChip category={skill.category} />
            <span
              className={
                'text-[10px] font-mono uppercase tracking-eyebrow px-1.5 py-0.5 rounded ' +
                methodColor(skill.method)
              }
            >
              {skill.method}
            </span>
          </div>
          <p className="text-sm text-claude-muted leading-relaxed max-w-2xl">
            {skill.description}
          </p>
        </header>

        <BasicInfoCard skill={skill} />

        <ProviderNote />

        <ParamsCard params={skill.params} />

        <RequestExampleCard example={skill.example} />

        <ResponseExampleCard />

        <div className="pt-2 flex items-center justify-between border-t border-claude-hairline">
          <Link
            href="/docs"
            className="text-sm text-claude-muted hover:text-claude-ink"
          >
            ← 返回文档首页
          </Link>
          <Link
            href={`/docs/sandbox?skill=${skill.id}`}
            className="inline-flex items-center gap-1.5 bg-claude-coral text-white text-sm font-medium px-4 py-2 rounded-md hover:bg-claude-coral/90 transition-colors"
          >
            在沙盒测试
            <span aria-hidden="true">→</span>
          </Link>
        </div>
      </div>
    </>
  )
}

function BasicInfoCard({ skill }: { skill: SkillMeta }) {
  return (
    <section
      aria-labelledby="basic-info-heading"
      className="bg-white border border-claude-hairline rounded-lg shadow-claude-soft"
    >
      <header className="px-5 py-3 border-b border-claude-hairline">
        <h3 id="basic-info-heading" className="text-sm font-medium text-claude-ink">
          基础信息
        </h3>
      </header>
      <dl className="px-5 py-4 grid grid-cols-1 sm:grid-cols-3 gap-x-6 gap-y-4 text-sm">
        <div>
          <dt className="text-xs text-claude-muted mb-1">HTTP 方法</dt>
          <dd className="font-mono text-claude-ink">{skill.method}</dd>
        </div>
        <div className="sm:col-span-2">
          <dt className="text-xs text-claude-muted mb-1">路径</dt>
          <dd className="font-mono text-claude-ink break-all">{skill.path}</dd>
        </div>
        <div>
          <dt className="text-xs text-claude-muted mb-1">计费提示</dt>
          <dd className="text-claude-ink">{skill.costCentsHint}</dd>
        </div>
        <div>
          <dt className="text-xs text-claude-muted mb-1">限流</dt>
          <dd className="text-claude-ink">{skill.rateLimit}</dd>
        </div>
        <div>
          <dt className="text-xs text-claude-muted mb-1">分类</dt>
          <dd className="text-claude-ink">{categoryLabel(skill.category)}</dd>
        </div>
      </dl>
    </section>
  )
}

// ProviderNote is rendered on every per-skill docs page to surface
// the Phase 4 user-configurable provider model. The skill itself
// doesn't pick a model — the resolver (internal/agents/resolver.go)
// reads the caller's configured credential at request time and
// wires the right protocol adapter (Anthropic Messages API or
// OpenAI Chat Completions API).
//
// We surface this on every page (not per-skill) so operators only
// have to learn the model once.
function ProviderNote() {
  return (
    <section
      aria-labelledby="provider-note-heading"
      className="bg-claude-surface border border-claude-hairline rounded-lg px-5 py-4"
    >
      <h3 id="provider-note-heading" className="text-sm font-medium text-claude-ink">
        模型与 Provider
      </h3>
      <p className="text-xs text-claude-muted mt-1.5 leading-relaxed">
        <strong className="text-claude-ink font-medium">Provider:</strong>{' '}
        在调用时根据当前 operator 配置的凭据解析。请先在{' '}
        <Link href="/credentials" className="text-claude-coral hover:underline">
          /credentials
        </Link>{' '}
        页面新增一条凭据, 选择 <code className="font-mono text-[11px] px-1 py-0.5 bg-white rounded">protocol</code>{' '}
        (Anthropic Messages API 或 OpenAI Chat Completions API)、填入{' '}
        <code className="font-mono text-[11px] px-1 py-0.5 bg-white rounded">model_name</code> 与{' '}
        <code className="font-mono text-[11px] px-1 py-0.5 bg-white rounded">api_key</code>,
        可选 <code className="font-mono text-[11px] px-1 py-0.5 bg-white rounded">base_url</code> 用于自托管或镜像地址。Key 以 AES-GCM 加密存储,
        切换或轮转在下次请求生效, 无需重启。
      </p>
      <p className="text-xs text-claude-muted mt-2 leading-relaxed">
        若想用 MiniMax 的 Anthropic 兼容接口跑文本, 把{' '}
        <code className="font-mono text-[11px] px-1 py-0.5 bg-white rounded">base_url</code>{' '}
        设为{' '}
        <code className="font-mono text-[11px] px-1 py-0.5 bg-white rounded">https://api.minimaxi.com</code>
        , <code className="font-mono text-[11px] px-1 py-0.5 bg-white rounded">protocol</code> 选 Anthropic,{' '}
        <code className="font-mono text-[11px] px-1 py-0.5 bg-white rounded">model_name</code>{' '}
        用 <code className="font-mono text-[11px] px-1 py-0.5 bg-white rounded">MiniMax-M2.7-highspeed</code>。
        媒体类 skill (封面/TTS/视频) 仍走 MiniMax 自带客户端, 不受本次改动影响。
      </p>
    </section>
  )
}

function ParamsCard({ params }: { params: ParamSchema[] }) {
  return (
    <section
      aria-labelledby="params-heading"
      className="bg-white border border-claude-hairline rounded-lg shadow-claude-soft"
    >
      <header className="px-5 py-3 border-b border-claude-hairline flex items-baseline justify-between">
        <h3 id="params-heading" className="text-sm font-medium text-claude-ink">
          请求参数
        </h3>
        <span className="text-xs text-claude-muted">{params.length} 个字段</span>
      </header>
      {params.length === 0 ? (
        <p className="px-5 py-6 text-sm text-claude-muted">
          这个 skill 不需要请求体, 直接调用即可。
        </p>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-left text-xs text-claude-muted border-b border-claude-hairline">
                <th scope="col" className="px-5 py-2 font-medium">名称</th>
                <th scope="col" className="px-5 py-2 font-medium">类型</th>
                <th scope="col" className="px-5 py-2 font-medium">必填</th>
                <th scope="col" className="px-5 py-2 font-medium">默认</th>
                <th scope="col" className="px-5 py-2 font-medium">说明</th>
              </tr>
            </thead>
            <tbody>
              {params.map((p) => (
                <tr key={p.name} className="border-b border-claude-hairline last:border-b-0 align-top">
                  <td className="px-5 py-3">
                    <div className="font-mono text-claude-ink">{p.name}</div>
                    <div className="text-xs text-claude-muted mt-0.5">{p.label}</div>
                  </td>
                  <td className="px-5 py-3 font-mono text-xs text-claude-muted">
                    {p.kind}
                  </td>
                  <td className="px-5 py-3 text-xs">
                    {p.required ? (
                      <span className="text-claude-coral">必填</span>
                    ) : (
                      <span className="text-claude-muted">选填</span>
                    )}
                  </td>
                  <td className="px-5 py-3 font-mono text-xs text-claude-ink">
                    {formatDefault(p.default)}
                  </td>
                  <td className="px-5 py-3 text-xs text-claude-muted leading-relaxed">
                    {p.help ?? '—'}
                    {p.serverOnly ? (
                      <div className="mt-1 text-[10px] text-claude-muted/80 italic">
                        server-only (会话侧派生, 不出现在沙盒表单)
                      </div>
                    ) : null}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  )
}

function RequestExampleCard({ example }: { example?: Record<string, unknown> }) {
  const body = example ?? {}
  return (
    <section
      aria-labelledby="request-example-heading"
      className="bg-white border border-claude-hairline rounded-lg shadow-claude-soft"
    >
      <header className="px-5 py-3 border-b border-claude-hairline">
        <h3 id="request-example-heading" className="text-sm font-medium text-claude-ink">
          请求示例
        </h3>
      </header>
      <pre className="px-5 py-4 text-xs font-mono text-claude-ink overflow-x-auto leading-relaxed">
        <code>{JSON.stringify(body, null, 2)}</code>
      </pre>
    </section>
  )
}

function ResponseExampleCard() {
  return (
    <section
      aria-labelledby="response-example-heading"
      className="bg-white border border-claude-hairline rounded-lg shadow-claude-soft"
    >
      <header className="px-5 py-3 border-b border-claude-hairline">
        <h3 id="response-example-heading" className="text-sm font-medium text-claude-ink">
          响应示例
        </h3>
      </header>
      <div className="px-5 py-6 text-sm text-claude-muted">
        调一次后看沙盒输出。响应 schema 在 <code className="font-mono text-xs px-1 py-0.5 bg-claude-surface rounded">apps/shared/</code> 里按 skill 维护, 沙盒跑通后会把真实响应回填到本页。
      </div>
    </section>
  )
}

function CategoryChip({ category }: { category: SkillMeta['category'] }) {
  return (
    <span className="text-[10px] uppercase tracking-eyebrow px-1.5 py-0.5 rounded bg-claude-surface text-claude-muted">
      {categoryLabel(category)}
    </span>
  )
}

function categoryLabel(category: SkillMeta['category']): string {
  switch (category) {
    case 'ai':
      return 'AI'
    case 'media':
      return '媒体'
    case 'admin':
      return '运营'
  }
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

function formatDefault(value: string | number | boolean | undefined): string {
  if (value === undefined) return '—'
  if (typeof value === 'string') return value
  return JSON.stringify(value)
}

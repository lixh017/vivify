import Link from 'next/link'
import { TopBar } from '@/components/TopBar'

// Custom not-found page for unknown skill ids. Next will render
// this when generateStaticParams() / getSkill() can't find the
// id. We give the operator a one-click path back to the index
// instead of the generic 404 stub.

export default function SkillNotFound() {
  return (
    <>
      <TopBar title="Skill 不存在" />
      <div className="p-6 max-w-2xl space-y-4">
        <div className="bg-white border border-claude-hairline rounded-lg p-6 shadow-claude-soft">
          <h2 className="text-base font-medium text-claude-ink">Skill 不存在</h2>
          <p className="text-sm text-claude-muted mt-2 leading-relaxed">
            这个 id 没有在 <code className="font-mono text-xs px-1 py-0.5 bg-claude-surface rounded">skills-registry.ts</code> 注册。
            可能是个笔误, 也可能是 skill 被重命名 / 下线了。
          </p>
          <div className="mt-4">
            <Link
              href="/docs"
              className="inline-flex items-center gap-1.5 text-sm text-claude-coral hover:underline"
            >
              <span aria-hidden="true">←</span>
              返回文档首页
            </Link>
          </div>
        </div>
      </div>
    </>
  )
}

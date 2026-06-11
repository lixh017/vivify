import { TopBar } from '@/components/TopBar'

// Reserve route. The URL exists so the future "multi-tenant org
// model" can be added without a redirect migration. The page
// is honest about the gap so an operator who finds the URL
// understands what's happening.
export default function OrgPage() {
  return (
    <>
      <TopBar title="组织" />
      <div className="p-6">
        <div className="bg-claude-surface-soft border border-claude-hairline rounded-lg p-6 max-w-xl">
          <span className="inline-block text-[10px] uppercase tracking-eyebrow text-claude-muted mb-2">
            Reserve route
          </span>
          <h2 className="text-base font-medium">多组织管理</h2>
          <p className="text-sm text-claude-body mt-2 leading-relaxed">
            当前为单用户模式。组织切换、多账号矩阵管理等功能会在客户需要
            多 operator / 多工作室协作时启动。
          </p>
          <p className="text-sm text-claude-muted mt-3">
            详见 <code className="bg-claude-surface-card px-1.5 py-0.5 rounded text-xs">docs/console-reserve-audit.md</code> (#46)。
          </p>
        </div>
      </div>
    </>
  )
}

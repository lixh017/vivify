import { TopBar } from '@/components/TopBar'

export default function MembersPage() {
  return (
    <>
      <TopBar title="成员" />
      <div className="p-6">
        <div className="bg-claude-surface-soft border border-claude-hairline rounded-lg p-6 max-w-xl">
          <span className="inline-block text-[10px] uppercase tracking-eyebrow text-claude-muted mb-2">
            Reserve route
          </span>
          <h2 className="text-base font-medium">成员管理</h2>
          <p className="text-sm text-claude-body mt-2 leading-relaxed">
            多 operator 协作（邀请、角色权限、SSO）会在需要时启动。
            单用户模式不暴露。
          </p>
        </div>
      </div>
    </>
  )
}

import { TopBar } from '@/components/TopBar'

export default function SettingsPage() {
  return (
    <>
      <TopBar title="设置" />
      <div className="p-6 space-y-4">
        <Card>
          <h2 className="text-base font-medium">账户设置</h2>
          <p className="text-sm text-claude-muted mt-1">
            个人信息、密码、API 端点 (NEXT_PUBLIC_API_URL)。
          </p>
        </Card>
        <Card>
          <h2 className="text-base font-medium">系统信息</h2>
          <p className="text-sm text-claude-muted mt-1">
            OPC version · 后端版本 · 数据库版本 · 部署时间。
          </p>
        </Card>
      </div>
    </>
  )
}

function Card({ children }: { children: React.ReactNode }) {
  return (
    <div className="bg-white border border-claude-hairline rounded-lg p-5 shadow-claude-soft">
      {children}
    </div>
  )
}

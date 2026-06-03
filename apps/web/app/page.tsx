export default function Home() {
  return (
    <div className="space-y-4">
      <h1 className="text-3xl font-bold">🐼 OPC 创作控制台</h1>
      <p className="text-gray-600">熊猫 IP · Phase 1</p>
      <div className="grid grid-cols-2 md:grid-cols-5 gap-4">
        <a href="/topics" className="p-4 bg-white rounded-lg shadow hover:shadow-md">
          <h2 className="font-semibold">📋 选题</h2>
          <p className="text-sm text-gray-500">Kanban</p>
        </a>
        <a href="/scripts" className="p-4 bg-white rounded-lg shadow hover:shadow-md">
          <h2 className="font-semibold">📝 脚本</h2>
          <p className="text-sm text-gray-500">库</p>
        </a>
        <a href="/calendar" className="p-4 bg-white rounded-lg shadow hover:shadow-md">
          <h2 className="font-semibold">📅 日历</h2>
          <p className="text-sm text-gray-500">排期</p>
        </a>
        <a href="/dashboard" className="p-4 bg-white rounded-lg shadow hover:shadow-md">
          <h2 className="font-semibold">📊 表现</h2>
          <p className="text-sm text-gray-500">Dashboard</p>
        </a>
        <a href="/knowledge" className="p-4 bg-white rounded-lg shadow hover:shadow-md">
          <h2 className="font-semibold">📚 知识库</h2>
          <p className="text-sm text-gray-500">SOP/笔记</p>
        </a>
      </div>
    </div>
  )
}

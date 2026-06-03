import './globals.css'
import type { Metadata } from 'next'

export const metadata: Metadata = {
  title: 'OPC',
  description: 'OPC 短视频创作平台',
}

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="zh-CN">
      <body className="bg-gray-50">
        <nav className="bg-white border-b border-gray-200 px-6 py-3">
          <div className="flex gap-6 text-sm font-medium">
            <a href="/topics" className="hover:text-blue-600">选题</a>
            <a href="/scripts" className="hover:text-blue-600">脚本</a>
            <a href="/calendar" className="hover:text-blue-600">日历</a>
            <a href="/dashboard" className="hover:text-blue-600">表现</a>
            <a href="/knowledge" className="hover:text-blue-600">知识库</a>
          </div>
        </nav>
        <main className="container mx-auto px-6 py-6">{children}</main>
      </body>
    </html>
  )
}

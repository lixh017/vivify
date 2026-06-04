import './globals.css'
import type { Metadata } from 'next'
import NavBar from './NavBar'

export const metadata: Metadata = {
  title: 'OPC',
  description: 'OPC 短视频创作平台',
}

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="zh-CN">
      <body className="bg-gray-50">
        <NavBar />
        <main className="container mx-auto px-3 md:px-6 py-4 md:py-6">
          {children}
        </main>
      </body>
    </html>
  )
}

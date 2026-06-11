import './globals.css'
import type { Metadata } from 'next'

export const metadata: Metadata = {
  title: 'OPC Console',
  description: 'OPC MCN 控制台',
}

// Console root layout. The chrome (sidebar + topbar) lives in the
// (console) route group's layout so unauthenticated routes
// (e.g. /login) can opt out of the shell.
export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="zh-CN">
      <body className="bg-claude-canvas text-claude-ink font-sans antialiased">
        {children}
      </body>
    </html>
  )
}

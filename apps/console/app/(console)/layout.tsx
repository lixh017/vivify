import { ReactNode } from 'react'
import { Sidebar } from '@/components/Sidebar'
import { TopBar } from '@/components/TopBar'

// The (console) route group is what makes the admin shell
// uniform across all six active routes + three reserve ones.
// Pages inside the group get the sidebar + topbar; pages
// outside (e.g. /login) opt out by living at app/login.
export default function ConsoleLayout({ children }: { children: ReactNode }) {
  return (
    <div className="flex min-h-screen">
      <Sidebar />
      <div className="flex-1 flex flex-col min-w-0">
        <div className="flex-1 flex flex-col">
          {children}
        </div>
      </div>
    </div>
  )
}

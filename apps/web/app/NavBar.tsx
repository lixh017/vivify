'use client'

import { useState, useEffect } from 'react'

interface NavLink {
  href: string
  label: string
  icon: string
}

const NAV_LINKS: NavLink[] = [
  { href: '/topics', label: '选题', icon: '📋' },
  { href: '/scripts', label: '脚本', icon: '📝' },
  { href: '/calendar', label: '日历', icon: '📅' },
  { href: '/dashboard', label: '表现', icon: '📊' },
  { href: '/knowledge', label: '知识库', icon: '📚' },
]

export default function NavBar() {
  const [open, setOpen] = useState(false)

  // Close the mobile menu when the viewport grows past the md breakpoint
  // or when the user starts navigating. The resize listener is throttled
  // by the fact that it only does work on a real width change.
  useEffect(() => {
    function handleResize() {
      if (window.innerWidth >= 768 && open) {
        setOpen(false)
      }
    }
    window.addEventListener('resize', handleResize)
    return () => window.removeEventListener('resize', handleResize)
  }, [open])

  return (
    <nav className="bg-white border-b border-gray-200 sticky top-0 z-30">
      <div className="container mx-auto px-3 md:px-6">
        <div className="flex items-center justify-between h-14 md:h-16">
          <a
            href="/"
            className="text-lg md:text-xl font-bold text-gray-900 flex items-center gap-2"
          >
            <span aria-hidden="true">🐼</span>
            <span>OPC</span>
          </a>

          {/* Desktop nav — horizontal, hidden on mobile */}
          <div className="hidden md:flex gap-6 text-sm font-medium">
            {NAV_LINKS.map((link) => (
              <a
                key={link.href}
                href={link.href}
                className="hover:text-blue-600 text-gray-700"
              >
                {link.icon} {link.label}
              </a>
            ))}
          </div>

          {/* Mobile hamburger button — hidden on desktop */}
          <button
            type="button"
            aria-label="切换导航菜单"
            aria-expanded={open}
            onClick={() => setOpen((v) => !v)}
            className="md:hidden inline-flex items-center justify-center p-2 rounded text-gray-600 hover:text-gray-900 hover:bg-gray-100"
          >
            {open ? (
              <svg
                xmlns="http://www.w3.org/2000/svg"
                className="h-6 w-6"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                aria-hidden="true"
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={2}
                  d="M6 18L18 6M6 6l12 12"
                />
              </svg>
            ) : (
              <svg
                xmlns="http://www.w3.org/2000/svg"
                className="h-6 w-6"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                aria-hidden="true"
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={2}
                  d="M4 6h16M4 12h16M4 18h16"
                />
              </svg>
            )}
          </button>
        </div>

        {/* Mobile drawer */}
        {open && (
          <div className="md:hidden border-t border-gray-200 py-2 space-y-1">
            {NAV_LINKS.map((link) => (
              <a
                key={link.href}
                href={link.href}
                onClick={() => setOpen(false)}
                className="block px-3 py-2 rounded text-base font-medium text-gray-700 hover:bg-gray-100 hover:text-gray-900"
              >
                {link.icon} {link.label}
              </a>
            ))}
          </div>
        )}
      </div>
    </nav>
  )
}

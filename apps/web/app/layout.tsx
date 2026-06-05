import './globals.css'
import type { Metadata } from 'next'
import NavBar from './NavBar'
import { getLocale } from '@/lib/i18n'

export const metadata: Metadata = {
  title: 'OPC',
  description: 'OPC 短视频创作平台',
}

// Map our Locale to the BCP-47 language tag the <html lang="...">
// attribute expects. Kept tiny because it only branches over the two
// locales we ship in Phase 1.
const LANG_ATTR: Record<'zh' | 'en', string> = {
  zh: 'zh-CN',
  en: 'en',
}

export default function RootLayout({ children }: { children: React.ReactNode }) {
  // The middleware guarantees the cookie is set, so this read is
  // effectively free. We still wrap it in the i18n module so a
  // future locale drop-in (e.g. ja, ko) only needs to be added in
  // one place.
  const locale = getLocale()
  return (
    <html lang={LANG_ATTR[locale]}>
      <body className="bg-claude-canvas text-claude-ink font-sans antialiased">
        <NavBar />
        <main>{children}</main>
      </body>
    </html>
  )
}

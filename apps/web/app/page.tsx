interface ShortcutCard {
  href: string
  icon: string
  title: string
  subtitle: string
}

const SHORTCUTS: ShortcutCard[] = [
  { href: '/topics', icon: '📋', title: '选题', subtitle: 'Kanban' },
  { href: '/scripts', icon: '📝', title: '脚本', subtitle: '库' },
  { href: '/calendar', icon: '📅', title: '日历', subtitle: '排期' },
  { href: '/dashboard', icon: '📊', title: '表现', subtitle: 'Dashboard' },
  { href: '/knowledge', icon: '📚', title: '知识库', subtitle: 'SOP/笔记' },
]

export default function Home() {
  return (
    <div className="space-y-4 md:space-y-6">
      <div>
        <h1 className="text-2xl md:text-3xl font-bold">🐼 OPC 创作控制台</h1>
        <p className="text-sm md:text-base text-gray-600 mt-1">
          熊猫 IP · Phase 1
        </p>
      </div>
      <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-5 gap-3 md:gap-4">
        {SHORTCUTS.map((card) => (
          <a
            key={card.href}
            href={card.href}
            className="p-3 md:p-4 bg-white rounded-lg shadow hover:shadow-md transition-shadow"
          >
            <h2 className="font-semibold text-sm md:text-base">
              {card.icon} {card.title}
            </h2>
            <p className="text-xs md:text-sm text-gray-500 mt-1">
              {card.subtitle}
            </p>
          </a>
        ))}
      </div>
    </div>
  )
}

import { promises as fs } from 'node:fs'
import path from 'node:path'
import { notFound } from 'next/navigation'
import Link from 'next/link'
import matter from 'gray-matter'
import { MDXRemote } from 'next-mdx-remote/rsc'
import { TopBar } from '@/components/TopBar'

// /docs/operations/[slug] renders the operator-facing reference
// docs (auth / rate-limits / error-codes). Unlike /docs/skills/[id]
// — which is data-driven from skills-registry.ts — these pages
// are hand-authored markdown in apps/console/docs/operations/.
//
// We treat the markdown as content, not code: the frontmatter
// (title, description, order) is parsed with gray-matter, and the
// body is compiled by next-mdx-remote's RSC entry. No custom MDX
// components are registered yet — if we need callouts or skill
// embeds later, we add a `components` prop to MDXRemote.
//
// Tailwind doesn't ship a `prose` plugin by default, so we style
// the rendered HTML with the `opc-prose` utility class defined
// in globals.css. That keeps the look aligned with the rest of
// the console without dragging in @tailwindcss/typography.

interface PageProps {
  params: { slug: string }
}

interface DocFrontmatter {
  title?: string
  description?: string
  order?: number
}

// Hard-code the valid slugs. Adding a new doc is a one-line change
// here + the .md file in docs/operations/. We intentionally do NOT
// scan the directory at build time: it would silently pick up
// editor scratch files (auth.md.bak, .DS_Store) and the contract
// for "what's published" would live in two places.
const VALID_SLUGS = ['auth', 'rate-limits', 'error-codes'] as const
type ValidSlug = (typeof VALID_SLUGS)[number]

function isValidSlug(slug: string): slug is ValidSlug {
  return (VALID_SLUGS as readonly string[]).includes(slug)
}

export function generateStaticParams(): { slug: string }[] {
  return VALID_SLUGS.map((slug) => ({ slug }))
}

interface DocMeta {
  title: string
  description: string
}

function docTitle(slug: ValidSlug): string {
  switch (slug) {
    case 'auth':
      return '鉴权'
    case 'rate-limits':
      return '限流'
    case 'error-codes':
      return '错误码'
  }
}

async function loadDoc(slug: ValidSlug): Promise<{ body: string; meta: DocMeta }> {
  // Resolve relative to the console app, not process.cwd(), so the
  // page works in monorepo dev (cwd = repo root) and in prod
  // (cwd = apps/console) the same way.
  const filePath = path.join(process.cwd(), 'docs', 'operations', `${slug}.md`)
  const raw = await fs.readFile(filePath, 'utf-8')
  const parsed = matter(raw)
  const fm = parsed.data as DocFrontmatter
  return {
    body: parsed.content,
    meta: {
      title: fm.title ?? docTitle(slug),
      description: fm.description ?? '',
    },
  }
}

export default async function OperationDocPage({ params }: PageProps) {
  if (!isValidSlug(params.slug)) {
    notFound()
  }
  const { body, meta } = await loadDoc(params.slug)

  return (
    <>
      <TopBar title={meta.title} />
      <div className="p-6 space-y-6 max-w-4xl">
        <nav className="text-xs text-claude-muted">
          <Link href="/docs" className="hover:text-claude-ink">
            接入文档
          </Link>
          <span className="mx-1.5">/</span>
          <span className="text-claude-ink">{meta.title}</span>
        </nav>

        <article className="opc-prose">
          <MDXRemote source={body} />
        </article>

        <div className="pt-2 border-t border-claude-hairline">
          <Link href="/docs" className="text-sm text-claude-muted hover:text-claude-ink">
            ← 返回文档首页
          </Link>
        </div>
      </div>
    </>
  )
}

'use client'

import { useCallback } from 'react'

interface ScrollToFeaturesLinkProps {
  className?: string
  children: React.ReactNode
}

export default function ScrollToFeaturesLink({
  className,
  children,
}: ScrollToFeaturesLinkProps): React.ReactElement {
  const handleClick = useCallback(
    (event: React.MouseEvent<HTMLAnchorElement>): void => {
      // Always preventDefault so the browser does not paint the #features
      // hash into the URL before the smooth scroll kicks in — the
      // hash flicker was the visible QA bug.
      event.preventDefault()
      const target = document.getElementById('features')
      if (target) {
        target.scrollIntoView({ behavior: 'smooth', block: 'start' })
      }
    },
    [],
  )

  return (
    <a href="#features" onClick={handleClick} className={className}>
      {children}
    </a>
  )
}

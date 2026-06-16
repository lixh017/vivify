import type { Config } from 'tailwindcss'

const config: Config = {
  content: [
    './app/**/*.{js,ts,jsx,tsx,mdx}',
    './components/**/*.{js,ts,jsx,tsx,mdx}',
  ],
  theme: {
    extend: {
      // Inherit the demo's color tokens so the two apps feel like
      // one product. The console leans harder on the dark surfaces
      // (sidebar + nav) to set the "ops" tone.
      colors: {
        'claude-coral': '#cc785c',
        'claude-coral-active': '#a9583e',
        'claude-coral-disabled': '#e6dfd8',
        'claude-ink': '#141413',
        'claude-body': '#3d3d3a',
        'claude-muted': '#6c6a64',
        'claude-muted-soft': '#8e8b82',
        'claude-canvas': '#faf9f5',
        'claude-surface-soft': '#f5f0e8',
        'claude-surface-card': '#efe9de',
        'claude-surface-cream-strong': '#e8e0d2',
        'claude-hairline': '#e6dfd8',
        'claude-hairline-soft': '#ebe6df',
        'claude-surface-dark': '#181715',
        'claude-surface-dark-elevated': '#252320',
        'claude-surface-dark-soft': '#1f1e1b',
        'claude-on-primary': '#ffffff',
        'claude-on-dark': '#faf9f5',
        'claude-on-dark-soft': '#a09d96',
        'claude-accent-teal': '#5db8a6',
        'claude-accent-teal-active': '#3f8a7c',
        'claude-accent-amber': '#e8a55a',
        'claude-accent-amber-active': '#c08a45',
        // Violet accent for the OpenAI provider chip — sits between
        // the warm Anthropic coral and the cool Volcengine teal so
        // the four-provider badge palette stays balanced without
        // overlapping with any existing accent.
        'claude-accent-violet': '#8b6fb5',
        'claude-accent-violet-active': '#6d5294',
        'claude-success': '#5db872',
        'claude-warning': '#d4a017',
        'claude-error': '#c64545',
      },
      fontFamily: {
        sans: ['"Inter"', 'system-ui', 'sans-serif'],
        mono: ['"JetBrains Mono"', 'ui-monospace', 'monospace'],
      },
      borderRadius: {
        sm: '6px',
        md: '8px',
        lg: '12px',
        xl: '16px',
        pill: '9999px',
      },
      boxShadow: {
        'claude-soft': '0 1px 3px rgba(20, 20, 19, 0.08)',
      },
    },
  },
  plugins: [],
}
export default config

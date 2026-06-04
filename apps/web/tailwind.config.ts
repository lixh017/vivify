import type { Config } from 'tailwindcss'

const config: Config = {
  content: [
    './app/**/*.{js,ts,jsx,tsx,mdx}',
    './components/**/*.{js,ts,jsx,tsx,mdx}',
  ],
  theme: {
    extend: {
      colors: {
        // Primary palette — Anthropic warm coral
        'claude-coral': '#cc785c',
        'claude-coral-active': '#a9583e',
        'claude-coral-disabled': '#e6dfd8',
        // Ink (text)
        'claude-ink': '#141413',
        'claude-body': '#3d3d3a',
        'claude-muted': '#6c6a64',
        'claude-muted-soft': '#8e8b82',
        // Canvas / surfaces
        'claude-canvas': '#faf9f5',
        'claude-surface-soft': '#f5f0e8',
        'claude-surface-card': '#efe9de',
        'claude-surface-cream-strong': '#e8e0d2',
        'claude-hairline': '#e6dfd8',
        'claude-hairline-soft': '#ebe6df',
        // Dark surfaces (code editor / detail panels)
        'claude-surface-dark': '#181715',
        'claude-surface-dark-elevated': '#252320',
        'claude-surface-dark-soft': '#1f1e1b',
        // On-tonal helpers
        'claude-on-primary': '#ffffff',
        'claude-on-dark': '#faf9f5',
        'claude-on-dark-soft': '#a09d96',
        // Accents
        'claude-accent-teal': '#5db8a6',
        'claude-accent-amber': '#e8a55a',
        // Semantic
        'claude-success': '#5db872',
        'claude-warning': '#d4a017',
        'claude-error': '#c64545',
      },
      fontFamily: {
        // Display: Fraunces is the closest free Google Font to Copernicus
        serif: ['"Fraunces"', '"Playfair Display"', 'Georgia', 'serif'],
        // Body / UI: Inter is the closest free substitute for StyreneB
        sans: ['"Inter"', 'system-ui', 'sans-serif'],
        mono: ['"JetBrains Mono"', 'ui-monospace', 'monospace'],
      },
      spacing: {
        // Claude section rhythm — 96px between major bands
        section: '96px',
      },
      borderRadius: {
        // Hierarchical rounded scale — modest, not full-pill by default
        sm: '6px',
        md: '8px',
        lg: '12px',
        xl: '16px',
        pill: '9999px',
      },
      letterSpacing: {
        // Display sizes use negative tracking (Copernicus reads off-brand without it)
        'display-xl': '-1.5px',
        'display-lg': '-1px',
        'display-md': '-0.5px',
        'display-sm': '-0.3px',
        'eyebrow': '1.5px',
      },
      maxWidth: {
        // Editorial content caps at ~1200px
        content: '1200px',
      },
      boxShadow: {
        // The system is shadowless by default; only one subtle hover-elevated
        // shadow is encoded for rare interactive states.
        'claude-soft': '0 1px 3px rgba(20, 20, 19, 0.08)',
      },
    },
  },
  plugins: [],
}
export default config

import type { Theme } from './index'

/** One CSS adapter for DaisyUI, charts, and read-only code surfaces. */
export function themeTokens(theme: Theme) {
  const c = theme.colors
  const light = theme.scheme === 'light'
  const content = light ? 'var(--color-base-100)' : 'var(--color-base-content)'
  return {
    'color-scheme': theme.scheme,
    '--color-base-100': c.canvas,
    '--color-base-200': c.panel,
    '--color-base-300': c.chrome,
    '--color-base-content': c.text,
    '--color-subtle': c.subtle,
    '--color-muted': c.muted,
    '--color-rose': c.rose,
    '--color-highlight-low': c.highlightLow,
    '--color-highlight-med': c.highlightMedium,
    '--color-highlight-high': c.highlightHigh,
    '--color-primary': c.primary,
    '--color-primary-content': 'var(--color-base-100)',
    '--color-secondary': c.secondary,
    '--color-secondary-content': content,
    '--color-accent': c.accent,
    '--color-accent-content': 'var(--color-base-100)',
    '--color-neutral': 'var(--color-muted)',
    '--color-neutral-content': 'var(--color-base-content)',
    '--color-info': 'var(--color-accent)',
    '--color-info-content': 'var(--color-accent-content)',
    '--color-success': 'var(--color-secondary)',
    '--color-success-content': 'var(--color-secondary-content)',
    '--color-warning': c.warning,
    '--color-warning-content': 'var(--color-base-100)',
    '--color-error': c.error,
    '--color-error-content': content,
    '--color-on-light': light
      ? 'var(--color-base-content)'
      : theme.id === 'rose-pine'
        ? 'var(--color-base-200)'
        : 'var(--color-base-100)',
    '--color-on-dark': light
      ? 'var(--color-base-100)'
      : 'var(--color-base-content)',
    '--waterfall-bar-opacity': light ? '0.8' : '1',
    '--waterfall-bar-label-color': light
      ? 'var(--color-base-content)'
      : 'var(--color-base-200)',
    '--waterfall-marker-mix-target': 'var(--color-base-content)',
    '--waterfall-marker-mix-strength': light ? '0%' : '40%',
    '--metric-series-line-opacity': light ? '0.6' : '1',
    '--waterfall-tick-mix-target': 'white',
    '--waterfall-tick-mix-strength': light ? '60%' : '45%',
    // Dawn's existing small-code colours preserve contrast at 13px.
    '--readonly-code-subtle': light
      ? 'var(--color-base-content)'
      : 'var(--color-subtle)',
    '--readonly-code-gold': light ? '#6d4f00' : 'var(--color-warning)',
    '--readonly-code-rose': light ? '#8f435b' : 'var(--color-rose)',
    '--readonly-code-foam': light
      ? 'var(--color-secondary)'
      : 'var(--color-accent)',
    '--readonly-code-iris': light ? '#6e577f' : 'var(--color-primary)',
  }
}

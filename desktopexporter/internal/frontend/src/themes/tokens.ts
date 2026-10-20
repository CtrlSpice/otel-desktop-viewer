import type { Theme } from './index'

/** One CSS adapter for DaisyUI, charts, and read-only code surfaces. */
export function themeTokens(theme: Theme) {
  const c = theme.colors
  const light = theme.scheme === 'light'
  const inverseContent = light ? '#ffffff' : 'var(--color-base-100)'
  const content = light ? inverseContent : 'var(--color-base-content)'
  return {
    'color-scheme': theme.scheme,
    '--color-base-100': c.canvas,
    '--color-base-200': c.panel,
    '--color-base-300': c.chrome,
    '--color-base-content': c.text,
    '--color-subtle': c.subtle,
    '--color-muted': c.muted,
    '--color-rose': c.rose,
    '--color-code-literal': 'var(--color-primary)',
    '--color-highlight-low': c.highlightLow,
    '--color-highlight-med': c.highlightMedium,
    '--color-highlight-high': c.highlightHigh,
    '--color-primary': c.primary,
    '--color-primary-content': inverseContent,
    '--color-secondary': c.secondary,
    '--color-secondary-content': content,
    '--color-accent': c.accent,
    '--color-accent-content': inverseContent,
    '--color-neutral': 'var(--color-muted)',
    '--color-neutral-content': 'var(--color-base-content)',
    '--color-info': 'var(--color-accent)',
    '--color-info-content': 'var(--color-accent-content)',
    '--color-success': 'var(--color-secondary)',
    '--color-success-content': 'var(--color-secondary-content)',
    '--color-warning': c.warning,
    '--color-warning-content': inverseContent,
    '--color-error': c.error,
    '--color-error-content': inverseContent,
    '--color-on-light': light
      ? 'var(--color-base-content)'
      : theme.id === 'rose-pine'
        ? 'var(--color-base-200)'
        : 'var(--color-base-100)',
    '--color-on-dark': light
      ? 'var(--color-base-100)'
      : 'var(--color-base-content)',
    // Waterfall tree geometry.
    '--waterfall-gutter-col': '22px',
    '--tree-count-width': '22px',
    '--waterfall-gutter-hub': '8px',
    '--waterfall-gutter-leaf': '8px',
    '--waterfall-bar-label-color': light
      ? 'var(--color-base-content)'
      : 'var(--color-base-200)',
    '--waterfall-marker-mix-target': 'var(--color-base-content)',
    '--waterfall-marker-mix-strength': light ? '0%' : '40%',
    '--metric-series-line-opacity': light ? '0.6' : '1',
    '--waterfall-tick-mix-target': 'white',
    '--waterfall-tick-mix-strength': light ? '60%' : '45%',
  }
}

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
    '--color-sakura': c.sakura,
    ...Object.fromEntries(
      Object.entries(theme.syntax).map(([name, color]) => [
        `--color-code-${name}`,
        color,
      ])
    ),
    '--color-code-literal': 'var(--color-code-sumi)',
    '--color-highlight-low': c.highlightLow,
    '--color-highlight-med': c.highlightMedium,
    '--color-highlight-high': c.highlightHigh,
    '--color-input': 'var(--color-base-200)',
    '--color-border': 'var(--color-highlight-low)',
    '--color-primary': c.primary,
    '--color-secondary': c.secondary,
    '--color-accent': c.accent,
    '--color-primary-content': inverseContent,
    '--color-secondary-content': content,
    '--color-accent-content': inverseContent,
    '--color-neutral': 'var(--color-muted)',
    '--color-neutral-content': 'var(--color-base-content)',
    '--color-info': c.info,
    '--color-info-content': 'var(--color-accent-content)',
    '--color-success': c.success,
    '--color-success-content': 'var(--color-secondary-content)',
    '--color-warning': c.warning,
    '--color-warning-content': inverseContent,
    '--color-error': c.error,
    '--color-error-content': inverseContent,
    '--color-on-light': light
      ? 'var(--color-base-content)'
      : 'var(--color-base-100)',
    '--color-on-dark': light
      ? 'var(--color-base-100)'
      : 'var(--color-base-content)',
    // Waterfall tree geometry.
    '--waterfall-gutter-col': '18px',
    '--tree-count-width': '22px',
    '--waterfall-gutter-hub': '8px',
    '--waterfall-gutter-leaf': '8px',
    '--metric-series-line-opacity': light ? '0.6' : '1',
  }
}

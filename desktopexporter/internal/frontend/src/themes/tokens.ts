import type { Theme } from './index'
import {
  accentInkColors,
  DEFAULT_ACCENT_INK,
  type AccentInkId,
} from './accent-inks'

export function accentInkTokens(theme: Theme, ink: AccentInkId) {
  const accent = accentInkColors(ink, theme.id)
  return {
    '--color-primary': accent.colors.primary,
    '--color-secondary': accent.colors.secondary,
    '--color-accent': accent.colors.accent,
    ...Object.fromEntries(
      Object.entries(accent.syntax).map(([name, color]) => [
        `--color-code-${name}`,
        color,
      ])
    ),
  }
}

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
    '--waterfall-bar-label-color': light
      ? 'var(--color-base-content)'
      : 'var(--color-base-200)',
    '--waterfall-marker-mix-target': 'var(--color-base-content)',
    '--waterfall-marker-mix-strength': light ? '0%' : '40%',
    '--metric-series-line-opacity': light ? '0.6' : '1',
    '--waterfall-tick-mix-target': 'white',
    '--waterfall-tick-mix-strength': light ? '60%' : '45%',
    ...accentInkTokens(theme, DEFAULT_ACCENT_INK),
  }
}

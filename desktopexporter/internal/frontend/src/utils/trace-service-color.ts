import { categoricalPalette } from './chart-palette'

/** Stable service colours shared by trace summaries and multi-service waterfalls. */
export function traceServiceColor(serviceName: string, theme: string): string {
  const palette = categoricalPalette(25, 'iris', theme)
  let hash = 2166136261
  for (let i = 0; i < serviceName.length; i++) {
    hash = Math.imul(hash ^ serviceName.charCodeAt(i), 16777619)
  }
  return palette[(hash >>> 0) % palette.length]!
}

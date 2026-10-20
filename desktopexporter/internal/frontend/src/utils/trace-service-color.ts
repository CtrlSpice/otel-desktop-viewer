import { traceSpanPalette } from './chart-palette'
import { TRACE_SERIES } from '@/themes'
import type { SpanTiming } from '@/types/api-types'

export function spanTimingColor(span: SpanTiming, theme: string): string {
  return span.statusCodeValue === 2
    ? 'var(--color-error)'
    : traceServiceColor(span.serviceName, theme)
}

/** Stable service colours shared by trace summaries and multi-service waterfalls. */
export function traceServiceColor(serviceName: string, theme: string): string {
  const palette = traceSpanPalette(TRACE_SERIES.length, theme)
  let hash = 2166136261
  for (let i = 0; i < serviceName.length; i++) {
    hash = Math.imul(hash ^ serviceName.charCodeAt(i), 16777619)
  }
  return palette[(hash >>> 0) % palette.length]!
}

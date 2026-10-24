import { getSeriesPalette } from '@/themes'
import { identityColorSlot } from './color-slots'
import type { SpanTiming } from '@/types/api-types'

export function spanTimingColor(span: SpanTiming, theme: string): string {
  return span.statusCodeValue === 2
    ? 'var(--color-error)'
    : traceServiceColor(span.serviceName, theme)
}

/** Stable service colours shared by trace summaries and multi-service waterfalls. */
export function traceServiceColor(serviceName: string, theme: string): string {
  return getSeriesPalette(theme)[identityColorSlot(serviceName, 'services')]!
}

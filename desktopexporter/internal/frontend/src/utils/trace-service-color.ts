import { getSeriesPalette } from '@/themes'
import { identityColorSlot } from './color-slots'
import type { SpanTiming } from '@/types/api-types'
import { graphColorOverrides } from '@/state/graph-color-overrides.svelte'

export function spanTimingColor(span: SpanTiming, theme: string): string {
  return span.statusCodeValue === 2
    ? 'var(--color-error)'
    : traceServiceColor(span.serviceName, theme)
}

/** Stable service colours shared by trace summaries and multi-service waterfalls. */
export function traceServiceColor(serviceName: string, theme: string): string {
  const automatic =
    getSeriesPalette(theme)[identityColorSlot(serviceName, 'services')]!
  return graphColorOverrides.get('services', serviceName)?.color ?? automatic
}

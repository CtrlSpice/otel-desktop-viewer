import { getSeriesPalette } from '@/themes'
import { identityColorSlot } from './color-slots'
import type { SpanTiming } from '@/types/api-types'
import { graphColorOverrides } from '@/state/graph-color-overrides.svelte'
import { serviceColorIdentity } from './service-color-identity'

export function spanTimingColor(span: SpanTiming, theme: string): string {
  return span.statusCodeValue === 2
    ? 'var(--color-error)'
    : traceServiceColor(span.serviceName, theme)
}

/** Stable service colours shared by trace summaries and multi-service waterfalls. */
export function traceServiceColor(
  serviceName: string,
  theme: string,
  colorKey?: string
): string {
  const identity = colorKey ?? serviceColorIdentity(serviceName)
  const automatic =
    getSeriesPalette(theme)[identityColorSlot(identity, 'services')]!
  return graphColorOverrides.get('services', identity)?.color ?? automatic
}

/** Single-service waterfall colours use exact span names, not service slots. */
export function spanNameColor(name: string, theme: string): string {
  return getSeriesPalette(theme)[identityColorSlot(name, 'span-names')]!
}

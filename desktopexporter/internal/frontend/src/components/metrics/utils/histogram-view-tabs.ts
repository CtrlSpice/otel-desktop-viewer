import type { HistogramTab } from '@/contexts/metric-view-context.svelte'
import type { PaneTab } from '@/components/shared/PaneHeader.svelte'

/** UI labels for histogram chart view tabs in the chart PaneHeader. */
export const HISTOGRAM_VIEW_TAB_OPTIONS: ReadonlyArray<{
  id: HistogramTab
  label: string
}> = [
  { id: 'heatmap', label: 'Heatmap' },
  { id: 'quantiles', label: 'Quantiles' },
  { id: 'aggregated', label: 'Aggregated' },
  { id: 'snapshot', label: 'Snapshot' },
]

export function histogramViewTabOptions(): typeof HISTOGRAM_VIEW_TAB_OPTIONS {
  return HISTOGRAM_VIEW_TAB_OPTIONS
}

/** Lift tabs for histogram chart views. */
export function histogramViewTabs(): PaneTab[] {
  return HISTOGRAM_VIEW_TAB_OPTIONS.map(o => ({
    id: o.id,
    label: o.label,
  }))
}

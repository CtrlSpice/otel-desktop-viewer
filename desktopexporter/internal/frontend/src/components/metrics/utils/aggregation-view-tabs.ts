import type { PaneTab } from '@/components/shared/PaneHeader.svelte'
import type { AggregationView } from '@/components/metrics/utils/aggregation'

/** UI labels for aggregation view tabs in the chart PaneHeader. */
export const AGGREGATION_VIEW_TAB_OPTIONS: ReadonlyArray<{
  value: AggregationView
  label: string
}> = [
  { value: 'raw', label: 'Raw' },
  { value: 'sum', label: 'Sum' },
  { value: 'avg', label: 'Average' },
  { value: 'rate', label: 'Rate' },
]

/** Lift tabs for views that are meaningful for the current metric. */
export function aggregationViewTabs(available: AggregationView[]): PaneTab[] {
  return AGGREGATION_VIEW_TAB_OPTIONS.filter(o =>
    available.includes(o.value)
  ).map(o => ({ id: o.value, label: o.label }))
}

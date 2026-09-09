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
  const tabs: PaneTab[] = []
  for (const option of AGGREGATION_VIEW_TAB_OPTIONS) {
    if (!available.includes(option.value)) continue
    tabs.push({ id: option.value, label: option.label })
  }
  return tabs
}

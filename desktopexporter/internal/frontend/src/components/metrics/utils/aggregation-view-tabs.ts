import type { PaneTab } from '@/components/shared/PaneHeader.svelte'
import type { AggregationView } from '@/components/metrics/utils/aggregation'

/** Combined lines have independent checkboxes; only Rate needs a separate view. */
export function aggregationViewTabs(available: AggregationView[]): PaneTab[] {
  return available.includes('rate')
    ? [
        { id: 'values', label: 'Values' },
        { id: 'rate', label: 'Rate' },
      ]
    : []
}

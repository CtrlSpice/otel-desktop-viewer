<script lang="ts">
  import type { MetricViewData } from '@/types/api-types'
  import { createMetricViewContext } from '@/features/metrics/metric-view-context.svelte'
  import TimeseriesPanel from '@/features/metrics/Detail/TimeseriesPanel.svelte'

  let { metric }: { metric: MetricViewData } = $props()
  createMetricViewContext(() => metric)
</script>

<TimeseriesPanel />

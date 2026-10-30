<script lang="ts">
  import type { MetricViewData } from '@/types/api-types'
  import { createMetricViewContext } from '@/contexts/metric-view-context.svelte'
  import TimeseriesPanel from '@/components/metrics/Detail/TimeseriesPanel.svelte'

  let { metric }: { metric: MetricViewData } = $props()
  createMetricViewContext(() => metric)
</script>

<TimeseriesPanel />

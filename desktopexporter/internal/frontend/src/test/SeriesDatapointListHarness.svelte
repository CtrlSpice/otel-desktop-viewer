<script lang="ts">
  import { createMetricViewContext } from '@/contexts/metric-view-context.svelte'
  import SeriesDatapointList from '@/components/metrics/Detail/SeriesDatapointList.svelte'
  import type { DataPoint } from '@/types/api-types'

  type Props = {
    datapoints: DataPoint[]
    expandDatapointId?: string
  }
  let { datapoints, expandDatapointId }: Props = $props()

  const ctx = createMetricViewContext(() => undefined)

  $effect(() => {
    if (expandDatapointId) ctx.expandedDatapoints.add(expandDatapointId)
  })
</script>

<SeriesDatapointList {datapoints} />

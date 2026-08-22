<script lang="ts">
  import { createMetricViewContext } from '@/contexts/metric-view-context.svelte'
  import SeriesDatapointList from '@/components/metrics/Detail/SeriesDatapointList.svelte'
  import type { DataPoint } from '@/types/api-types'

  type Props = {
    datapoints: DataPoint[]
    expandDatapointID?: string
  }
  let { datapoints, expandDatapointID }: Props = $props()

  const ctx = createMetricViewContext(() => undefined)

  $effect(() => {
    if (expandDatapointID) ctx.expandedDatapoints.add(expandDatapointID)
  })
</script>

<SeriesDatapointList {datapoints} />

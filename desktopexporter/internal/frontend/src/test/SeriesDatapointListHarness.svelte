<script lang="ts">
  import { untrack } from 'svelte'
  import {
    createMetricViewContext,
    type MetricViewContext,
  } from '@/contexts/metric-view-context.svelte'
  import SeriesDatapointList from '@/components/metrics/Detail/SeriesDatapointList.svelte'
  import type { DataPoint, MetricData } from '@/types/api-types'

  type Props = {
    datapoints: DataPoint[]
    unit?: string
    expandDatapointID?: string
    oncontext?: (ctx: MetricViewContext) => void
  }
  let { datapoints, unit = '1', expandDatapointID, oncontext }: Props = $props()

  let metric = $derived<MetricData>({
    id: 'metric-list-harness',
    name: 'metric-list-harness',
    description: '',
    metadata: [],
    unit,
    metricType: datapoints[0]?.metricType ?? 'Empty',
    resourceDroppedAttributesCount: 0,
    resource: { attributes: [], droppedAttributesCount: 0 },
    scopeName: '',
    scopeVersion: '',
    scopeDroppedAttributesCount: 0,
    scope: {
      name: '',
      version: '',
      attributes: [],
      droppedAttributesCount: 0,
    },
    timeseries: [
      {
        attributesKey: 'series-1',
        attributes: [],
        resource: { attributes: [], droppedAttributesCount: 0 },
        // The component receives the separately fetched, unreduced rows.
        datapoints: [],
        stats: null,
        datapointCount: datapoints.length,
        lastSeenNs: datapoints.at(-1)?.timestamp ?? null,
        views: null,
        rateStats: null,
        sparkline: null,
      },
    ],
    datapointCount: datapoints.length,
    boundsMismatch: null,
    lastSeenNs: datapoints.at(-1)?.timestamp ?? null,
    window: {
      requested: { startNs: null, endNs: null },
      effective: {
        startNs: datapoints[0]?.timestamp ?? null,
        endNs: datapoints.at(-1)?.timestamp ?? null,
      },
    },
  })

  const ctx = createMetricViewContext(
    () => metric,
    () => null,
    () => null,
    () => null,
    seriesKey => (seriesKey === 'series-1' ? datapoints : undefined)
  )
  untrack(() => ctx.seedForMetric(metric))
  untrack(() => oncontext?.(ctx))

  $effect(() => {
    if (expandDatapointID) ctx.expandedDatapoints.add(expandDatapointID)
  })
</script>

<SeriesDatapointList {datapoints} />

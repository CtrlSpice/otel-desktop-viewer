<script lang="ts">
  import { Chart, Cell, Axis, Highlight, Layer, Tooltip } from 'layerchart'
  import { scaleBand, scaleQuantize } from 'd3-scale'
  import { schemeYlOrRd } from 'd3-scale-chromatic'
  import { extent } from 'd3-array'
  import { telemetryAPI } from '@/services/telemetry-service'
  import type {
    BucketSeriesMode,
    BucketSeriesPoint,
    HistogramBucketPoint,
    ExpHistogramBucketPoint,
  } from '@/types/api-types'

  type HeatmapDatum = {
    time: string
    bucket: string
    count: number
  }

  type Props = {
    metricID: string
    mode: BucketSeriesMode
    startTimeMs: number
    endTimeMs: number
    maxPoints?: number
    height?: number
  }

  let {
    metricID,
    mode,
    startTimeMs,
    endTimeMs,
    maxPoints = 100,
    height = 300,
  }: Props = $props()

  let points = $state<BucketSeriesPoint[]>([])
  let loading = $state(false)
  let error = $state<string | null>(null)

  $effect(() => {
    const id = metricID
    const m = mode
    const start = startTimeMs
    const end = endTimeMs
    const mp = maxPoints
    let cancelled = false

    loading = true
    error = null
    points = []

    telemetryAPI
      .getMetricBucketSeries(id, m, start, end, mp)
      .then(result => {
        if (!cancelled) {
          points = result
          loading = false
        }
      })
      .catch(err => {
        if (!cancelled) {
          console.warn('getMetricBucketSeries failed:', err)
          error = err instanceof Error ? err.message : String(err)
          loading = false
        }
      })

    return () => {
      cancelled = true
    }
  })

  function tsToMs(ts: bigint): number {
    return Number(ts / 1_000_000n)
  }

  function formatTime(ms: number): string {
    return new Date(ms).toLocaleTimeString(undefined, {
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    })
  }

  function formatBound(v: number): string {
    if (v === 0) return '0'
    if (Math.abs(v) >= 1000) return v.toExponential(1)
    if (Math.abs(v) < 0.01) return v.toExponential(1)
    return v.toPrecision(3)
  }

  let heatmapData = $derived.by((): HeatmapDatum[] => {
    if (points.length === 0) return []
    const first = points[0]
    if (first.kind === 'histogram') {
      return buildHistogramData(points as HistogramBucketPoint[])
    }
    return buildExpHistogramData(points as ExpHistogramBucketPoint[])
  })

  function buildHistogramData(pts: HistogramBucketPoint[]): HeatmapDatum[] {
    const data: HeatmapDatum[] = []
    for (const pt of pts) {
      const time = formatTime(tsToMs(pt.timestamp))
      const bounds = pt.bounds
      const counts = pt.counts
      for (let i = 0; i < counts.length; i++) {
        let label: string
        if (i === 0) {
          label = bounds.length > 0 ? `≤${formatBound(bounds[0])}` : '0'
        } else if (i < bounds.length) {
          label = formatBound((bounds[i - 1] + bounds[i]) / 2)
        } else {
          label = bounds.length > 0 ? `≥${formatBound(bounds[bounds.length - 1])}` : '0'
        }
        data.push({ time, bucket: label, count: counts[i] })
      }
    }
    return data
  }

  function buildExpHistogramData(pts: ExpHistogramBucketPoint[]): HeatmapDatum[] {
    const data: HeatmapDatum[] = []
    for (const pt of pts) {
      const time = formatTime(tsToMs(pt.timestamp))
      const base = Math.pow(2, Math.pow(2, -pt.scale))

      if (pt.zeroCount > 0) {
        data.push({ time, bucket: '0', count: pt.zeroCount })
      }

      for (let i = 0; i < pt.positiveCounts.length; i++) {
        const idx = pt.positiveOffset + i
        const mid = (Math.pow(base, idx) + Math.pow(base, idx + 1)) / 2
        data.push({ time, bucket: formatBound(mid), count: pt.positiveCounts[i] })
      }

      for (let i = 0; i < pt.negativeCounts.length; i++) {
        const idx = pt.negativeOffset + i
        const mid = (-Math.pow(base, idx + 1) + -Math.pow(base, idx)) / 2
        data.push({ time, bucket: formatBound(mid), count: pt.negativeCounts[i] })
      }
    }
    return data
  }

  // Ordered domain arrays for band scales — preserve time ordering and
  // bucket ordering (by numeric value, ascending bottom-to-top).
  let timeDomain = $derived.by(() => {
    const seen = new Map<string, number>()
    for (const d of heatmapData) {
      if (!seen.has(d.time)) seen.set(d.time, heatmapData.indexOf(d))
    }
    return [...seen.keys()]
  })

  let bucketDomain = $derived.by(() => {
    const seen = new Set<string>()
    const ordered: string[] = []
    for (const d of heatmapData) {
      if (!seen.has(d.bucket)) {
        seen.add(d.bucket)
        ordered.push(d.bucket)
      }
    }
    return ordered.reverse()
  })

  let countExtent = $derived(
    extent(heatmapData, d => d.count) as [number, number]
  )

  // Show at most ~6-8 evenly spaced labels to avoid clutter.
  let visibleTimeTicks = $derived.by(() => {
    const n = timeDomain.length
    if (n <= 8) return timeDomain
    const step = Math.ceil(n / 7)
    return timeDomain.filter((_, i) => i % step === 0)
  })

  let visibleBucketTicks = $derived.by(() => {
    const n = bucketDomain.length
    if (n <= 8) return bucketDomain
    const step = Math.ceil(n / 7)
    return bucketDomain.filter((_, i) => i % step === 0)
  })

</script>

{#if loading}
  <div
    class="flex items-center justify-center text-base-content/40 text-sm"
    style:height="{height}px"
  >
    Loading heatmap…
  </div>
{:else if error}
  <div
    class="flex items-center justify-center text-error/60 text-sm"
    style:height="{height}px"
  >
    {error}
  </div>
{:else if heatmapData.length === 0}
  <div
    class="flex items-center justify-center text-base-content/40 text-sm"
    style:height="{height}px"
  >
    No bucket data in range
  </div>
{:else}
  <div class="heatmap-wrapper" style:height="{height}px">
    <Chart
      data={heatmapData}
      x="time"
      xScale={scaleBand().padding(0)}
      xDomain={timeDomain}
      y="bucket"
      yScale={scaleBand().padding(0)}
      yDomain={bucketDomain}
      c="count"
      cScale={scaleQuantize()}
      cDomain={countExtent}
      cRange={schemeYlOrRd[9]}
      padding={{ top: 8, right: 8, bottom: 48, left: 56 }}
      {height}
      tooltipContext={{ mode: 'band' }}
    >
      <Layer>
        <Axis
          placement="bottom"
          rule
          ticks={visibleTimeTicks}
          tickLabelProps={{
            rotate: 315,
            textAnchor: 'end',
            verticalAnchor: 'middle',
            dy: 8,
          }}
        />
        <Axis placement="left" rule ticks={visibleBucketTicks} />
        <Cell x="time" y="bucket" fill="count" />
        <Highlight area />
      </Layer>
      <Tooltip.Root>
        {#snippet children({ data }: { data: HeatmapDatum })}
          <Tooltip.Header class="text-center">{data.time}</Tooltip.Header>
          <Tooltip.List>
            <Tooltip.Item label="bucket" value={data.bucket} />
            <Tooltip.Separator />
            <Tooltip.Item label="count" value={data.count} format="integer" />
          </Tooltip.List>
        {/snippet}
      </Tooltip.Root>
    </Chart>
  </div>
{/if}

<style lang="postcss">
  @reference "../../app.css";

  .heatmap-wrapper {
    @apply w-full overflow-hidden rounded-lg;
  }

  .heatmap-wrapper :global(.lc-rect) {
    stroke: none;
    shape-rendering: crispEdges;
  }
</style>

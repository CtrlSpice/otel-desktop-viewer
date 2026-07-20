<script lang="ts">
  import { LineChart, Line, Tooltip, type ChartState } from 'layerchart'
  import { bisector } from 'd3-array'
  import { scaleTime } from 'd3-scale'
  import { curveStepAfter } from 'd3-shape'
  import { formatMetricValue, formatRateSlopeValue } from '@/components/metrics/utils/format-metric-value'
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import { formatDateTime } from '@/utils/time'
  import MetricChartEmpty from '@/components/metrics/Charts/MetricChartEmpty.svelte'
  import MetricChartPlot, {
    axisTime,
    axisValue,
    chartPadding,
    DEFAULT_METRIC_CHART_HEIGHT,
  } from '@/components/metrics/Charts/MetricChartPlot.svelte'
  import { chartNeutral } from '@/utils/chart-palette'
  import ChartAggregateSummaryRows from '@/components/metrics/Charts/ChartAggregateSummaryRows.svelte'
  import ChartSelectionLegend, {
    type SelectionLegendRow,
  } from '@/components/metrics/Charts/ChartSelectionLegend.svelte'
  import ChartTimeRangeHeader from '@/components/metrics/Charts/ChartTimeRangeHeader.svelte'
  import {
    AGG_KEY_ALL,
    AGG_KEY_SELECTED,
    AGG_KEY_TOTAL,
    buildAggregateSummaryRows,
    type AggregateLineKey,
    type AggregateSummaryRow,
    type AggregationView,
    rateSlopeViewSymbol,
    rateSlopeBucketSegment,
    seriesStatsFromPoints,
  } from '@/components/metrics/utils/aggregation'
  import type { ChartPoint, ChartTimeseries } from '@/types/metric-chart-types'

  /** Render order inside the Totals section: checked → all. */
  const AGG_TOTAL_ORDER: Record<string, number> = {
    [AGG_KEY_SELECTED]: 0,
    [AGG_KEY_ALL]: 1,
    [AGG_KEY_TOTAL]: 1,
  }

  function isAggregateKey(key: string): key is AggregateLineKey {
    return (
      key === AGG_KEY_SELECTED ||
      key === AGG_KEY_ALL ||
      key === AGG_KEY_TOTAL
    )
  }

  function aggregateSummaryRowsAt(
    keys: readonly string[],
    valueAt: (key: string) => number | undefined
  ): AggregateSummaryRow[] {
    return buildAggregateSummaryRows(
      aggregateKeysInOrder(keys),
      aggregationView,
      (key) => valueAt(key),
      formatMetricValue
    )
  }

  function aggregateKeysInOrder(keys: readonly string[]): AggregateLineKey[] {
    return keys
      .filter(isAggregateKey)
      .slice()
      .sort(
        (a, b) => (AGG_TOTAL_ORDER[a] ?? 99) - (AGG_TOTAL_ORDER[b] ?? 99)
      )
  }

  /** Nearest point at `x` — layerchart's default tooltip matches exact
   *  timestamps only, which breaks when raw and aggregate grids differ. */
  function nearestValueAt(
    points: readonly { date: Date; value: number }[],
    x: Date
  ): number | undefined {
    if (points.length === 0) return undefined
    const t = x.getTime()
    const i = bisector((p: { date: Date }) => p.date.getTime()).left(points, t)
    const lo = Math.max(0, i - 1)
    const hi = Math.min(points.length - 1, i)
    const dl = Math.abs(points[lo]!.date.getTime() - t)
    const dh = Math.abs(points[hi]!.date.getTime() - t)
    return (dl <= dh ? points[lo] : points[hi])!.value
  }

  function tooltipXDate(context: {
    tooltip: { data: unknown }
    x: (d: unknown) => unknown
    valueAxis: string
  }): Date | null {
    const d = context.tooltip.data
    if (d == null) return null
    const v = context.x(d)
    if (v instanceof Date) return v
    if (v != null) return new Date(v as string | number)
    return null
  }

  type Props = {
    /** Pre-grouped, already visibility-filtered timeseries. The caller
     *  (MetricViewContext via MetricChartView) drops timeseries the
     *  user has unchecked in the legend BEFORE handing the array down,
     *  so the chart trusts every entry here is meant to render. */
    timeseries: ChartTimeseries[]
    height?: number
    /** Timestamp (ns, as bigint) of a datapoint to highlight on the
     * chart. When set, draws a vertical rule at that x-coordinate so
     * a click on the datapoints list visually anchors to its point. */
    highlightedTimestamp?: bigint | null
    /** Attributes key of the timeseries owning the selected datapoint.
     *  When set alongside `highlightedTimestamp`, the chart draws a
     *  colored dot on the rule at that series's value. Aggregates
     *  always get dots too -- the selected series is the one we'd
     *  otherwise not be able to identify just from x-position. */
    selectedSeriesKey?: string | null
    /** Metric unit for the y-axis label (e.g. "ms", "bytes"). */
    unit?: string
    /** Per-timeseries color lookup keyed by `ChartTimeseries.key`. The
     *  caller owns this map (in practice it comes from the metric view
     *  context's color-index) so the chart line and the legend swatch
     *  for the same key always agree. Missing keys fall back to the
     *  neutral palette color. */
    colorByKey: ReadonlyMap<string, string>
    /** Message rendered when the chart has nothing to draw. The caller
     *  knows whether the cause is "no timeseries on this metric", "all
     *  unchecked", or "filtered to empty"; we just render whatever
     *  string it picks. Defaults to a generic fallback. */
    emptyMessage?: string
    /** Active chart view (Sum / Avg / Rate). Drives the aggregate-row
     *  glyph in the tooltip; omit or 'raw' when no aggregates render. */
    aggregationView?: AggregationView
    /** When false, min / max / avg selection overlays are hidden. */
    showStatOverlays?: boolean
    /** Rate slope (Δrate/Δt) at the selected bucket; chart-only tangent. */
    selectedRateSlope?: number | undefined
    /** Plotted data window; rendered as a permanent legend card above the plot. */
    timeRange?: { startMs: number; endMs: number } | null
    /** Chart point click → caller resolves to a datapoint and syncs
     *  the Series tab. Aggregate synthetic lines should no-op upstream. */
    onChartPointClick?: (seriesKey: string, clickedAt: Date) => void
  }

  let {
    timeseries,
    height = DEFAULT_METRIC_CHART_HEIGHT,
    highlightedTimestamp = null,
    selectedSeriesKey = null,
    unit = '',
    colorByKey,
    emptyMessage = 'No datapoints to chart',
    aggregationView = 'raw',
    showStatOverlays = true,
    selectedRateSlope = undefined,
    timeRange = null,
    onChartPointClick,
  }: Props = $props()

  const timeContext = getTimeContext()

  // Build the layerchart series array on the fly. Each entry carries
  // its own pre-grouped data so we don't re-traverse on every chart
  // re-render. Colour is looked up via the caller-provided `colorByKey`
  // map (keyed by attributesKey), not by position in this prop's array --
  // so toggling visibility never shifts a line's colour and the legend
  // swatch always matches the chart line for the same key.
  /** Per-series Spline props for cross-series aggregate lines.
   *
   *  - `stroke-dasharray`: visually distinguish aggregates from raw.
   *    Note: SVG attribute name (kebab-case) — Svelte does not
   *    translate JSX-style `strokeDasharray` to the SVG attribute.
   *  - `curve: curveStepAfter`: aggregates are per-bucket scalars
   *    ("average over this window", "rate during this window"). A
   *    smooth interpolation between bucket centers would imply
   *    continuity that doesn't exist; step-after draws a literal
   *    staircase that holds each bucket's value until the next
   *    bucket starts. Raw lines stay smooth because each raw point
   *    is an actual sample, not a window aggregate. */
  const AGG_LINE_PROPS = {
    'stroke-dasharray': '6 4',
    curve: curveStepAfter,
  } as const

  function seriesLineProps(key: string): Record<string, unknown> {
    if (isAggregateKey(key)) return { props: AGG_LINE_PROPS }
    return {}
  }

  let chartSeries = $derived.by(() => {
    return timeseries.map(ts => ({
      key: ts.key,
      label: ts.label,
      data: ts.points,
      color: colorByKey.get(ts.key) ?? chartNeutral(),
      ...seriesLineProps(ts.key),
    }))
  })

  function chartPointDate(data: unknown): Date | null {
    if (data == null || typeof data !== 'object') return null
    const row = data as { date?: unknown; x?: unknown }
    if (row.date instanceof Date) return row.date
    if (row.date != null) return new Date(row.date as string | number)
    if (row.x instanceof Date) return row.x
    if (row.x != null) return new Date(row.x as string | number)
    return null
  }

  function chartPointValue(data: unknown): number | undefined {
    if (data == null || typeof data !== 'object') return undefined
    const row = data as { value?: number; y?: number }
    const v = row.value ?? row.y
    return v !== undefined && Number.isFinite(v) ? v : undefined
  }

  let lineChartContext = $state<ChartState<ChartPoint> | undefined>(undefined)
  /** Plot area height after the selection legend claims its row. */
  let plotAreaHeight = $state(0)

  /** Plot y-value at the pointer, for disambiguating series at a shared x. */
  function pointerYDataValue(e: MouseEvent): number | undefined {
    const ctx = lineChartContext
    if (!ctx?.yScale?.invert) return undefined
    const root = (e.target as Element).closest('.lc-root-container')
    if (!root) return undefined
    const rect = root.getBoundingClientRect()
    const plotY = e.clientY - rect.top - ctx.padding.top
    const value = ctx.yScale.invert(plotY)
    return Number.isFinite(value) ? value : undefined
  }

  /** Pick the raw series whose value at `date` is closest to click y. */
  function seriesKeyAtPointerY(e: MouseEvent, date: Date): string | null {
    const clickY = pointerYDataValue(e)
    if (clickY === undefined) return null

    let bestKey: string | null = null
    let bestDist = Infinity
    for (const s of timeseries) {
      if (isAggregateKey(s.key)) continue
      const v = nearestValueAt(s.points, date)
      if (v === undefined || !Number.isFinite(v)) continue
      const dist = Math.abs(v - clickY)
      if (dist < bestDist) {
        bestDist = dist
        bestKey = s.key
      }
    }
    return bestKey
  }

  function highlightSeriesKey(details: unknown): string | null {
    if (details == null || typeof details !== 'object') return null
    const d = details as {
      point?: { seriesKey?: string }
      series?: { key?: string }
    }
    return d.point?.seriesKey ?? d.series?.key ?? null
  }

  /** Map a chart row back to a raw series key. With multiple series at the
   *  same x, match on y/value before falling back to per-series lookup. */
  function seriesKeyForChartPoint(
    data: unknown,
    explicitKey?: string | null
  ): string | null {
    if (explicitKey && !isAggregateKey(explicitKey)) {
      return explicitKey
    }

    const date = chartPointDate(data)
    if (date === null) return null
    const t = date.getTime()
    const clickedValue = chartPointValue(data)

    if (clickedValue !== undefined) {
      for (const s of timeseries) {
        if (isAggregateKey(s.key)) continue
        for (const p of s.points) {
          if (p.date.getTime() === t && p.value === clickedValue) {
            return s.key
          }
        }
      }

      // Same timestamp, closest value (bucket centers / float drift).
      let valueMatchKey: string | null = null
      let bestValueDist = Infinity
      for (const s of timeseries) {
        if (isAggregateKey(s.key)) continue
        for (const p of s.points) {
          if (p.date.getTime() !== t) continue
          const vd = Math.abs(p.value - clickedValue)
          if (vd < bestValueDist) {
            bestValueDist = vd
            valueMatchKey = s.key
          }
        }
      }
      if (valueMatchKey !== null) return valueMatchKey

      // Shared x: pick the series whose interpolated value is closest to
      // the clicked y (bisect / tooltip-area clicks).
      let nearestValueKey: string | null = null
      let nearestValueDist = Infinity
      for (const s of timeseries) {
        if (isAggregateKey(s.key)) continue
        const v = nearestValueAt(s.points, date)
        if (v === undefined || !Number.isFinite(v)) continue
        const dist = Math.abs(v - clickedValue)
        if (dist < nearestValueDist) {
          nearestValueDist = dist
          nearestValueKey = s.key
        }
      }
      if (nearestValueKey !== null) return nearestValueKey
    }

    // No y to disambiguate — nearest point within each series, then closest time.
    let bestKey: string | null = null
    let bestDist = Infinity
    for (const s of timeseries) {
      if (isAggregateKey(s.key)) continue
      for (const p of s.points) {
        const dist = Math.abs(p.date.getTime() - t)
        if (dist < bestDist) {
          bestDist = dist
          bestKey = s.key
        }
      }
    }
    return bestKey
  }

  function dispatchChartPointClick(
    e: MouseEvent,
    detail: unknown,
    explicitKey?: string | null,
    source: 'point' | 'plot' = 'plot'
  ) {
    if (!onChartPointClick) return
    const payload =
      typeof detail === 'object' && detail !== null && 'data' in detail
        ? (detail as { data: unknown }).data
        : detail
    const date = chartPointDate(payload)
    if (date === null) return

    const fromHighlight = explicitKey ?? highlightSeriesKey(detail)
    let key =
      fromHighlight && !isAggregateKey(fromHighlight)
        ? fromHighlight
        : source === 'plot'
          ? seriesKeyAtPointerY(e, date)
          : null
    if (!key) {
      key = seriesKeyForChartPoint(payload, fromHighlight)
    }
    if (!key || isAggregateKey(key)) return

    onChartPointClick(key, date)
  }

  /** Highlight circle click. LineChart types `{ data, series }`; Highlight
   *  runtime passes `{ point, data }` with `point.seriesKey`. */
  function handlePointClick(
    e: MouseEvent,
    details: { data: { x: unknown; y: unknown }; series: { key: string } }
  ) {
    const key = highlightSeriesKey(details) ?? details.series?.key
    if (key && isAggregateKey(key)) return
    e.stopPropagation()
    dispatchChartPointClick(
      e,
      details,
      highlightSeriesKey(details),
      'point'
    )
  }

  /** Chart-area click under bisect-x tooltip — disambiguate series by
   *  pointer y, not the bisected row's value (wrong with many series). */
  function handleTooltipClick(e: MouseEvent, detail: { data: unknown }) {
    dispatchChartPointClick(e, detail, null, 'plot')
  }

  // Total visible point count -- if every series is empty (or all
  // hidden) we render a placeholder instead of an empty chart frame
  // so the user knows the absence is real, not a load state.
  let visiblePointCount = $derived.by(() => {
    let n = 0
    for (const ts of chartSeries) n += ts.data.length
    return n
  })

  // Resolve the highlighted timestamp to a chart-domain Date so we can
  // place the vertical rule. Fall back to null when no highlight or
  // the value falls outside the loaded range.
  let highlightDate = $derived.by((): Date | null => {
    if (highlightedTimestamp === null || highlightedTimestamp === undefined) {
      return null
    }
    return new Date(Number(highlightedTimestamp / 1_000_000n))
  })

  // Rate view: the y-axis is in "<unit> per second," so append "/s"
  // to the displayed unit. OTLP's dimensionless marker is "1", which
  // reads as "1/s" — that's noisy, so collapse it to just "/s" so the
  // axis says something honest ("events per second") without the
  // literal "1" leaking through. Empty unit + rate also collapses to
  // "/s" for the same reason.
  let yAxisLabel = $derived.by((): string => {
    const u = unit.trim()
    if (aggregationView === 'rate') {
      if (u === '' || u === '1') return '/s'
      return `${u}/s`
    }
    return u || 'value'
  })

  /** Series the selection rule should drop colored dots on. Always
   *  empty when nothing is selected. Otherwise: the user-selected
   *  series (if it's still in `chartSeries`) plus every aggregate
   *  currently rendered. Aggregates included unconditionally so users
   *  can read each aggregate's value at the selected x-coordinate
   *  without hover-tracking the cursor. Values resolved via the same
   *  `nearestValueAt` lookup the tooltip uses, so raw and aggregate
   *  grids that disagree on exact timestamps still produce dots. */
  type SelectionDot = {
    key: string
    color: string
    value: number
    isSelected: boolean
  }
  /** First point at an extremum (earliest timestamp on ties). */
  function extremumPoint(
    points: readonly ChartPoint[],
    kind: 'min' | 'max'
  ): ChartPoint | undefined {
    if (points.length === 0) return undefined
    let best = points[0]!
    for (const p of points) {
      if (kind === 'min') {
        if (p.value < best.value) best = p
      } else if (p.value > best.value) {
        best = p
      }
    }
    return best
  }

  const SERIES_STAT_LABEL: Record<'min' | 'max' | 'avg' | 'slope', string> = {
    min: 'min',
    max: 'max',
    avg: 'avg',
    slope: rateSlopeViewSymbol(),
  }

  /** Hover target thickness (px) for min/max/avg/slope overlay guides. */
  const STAT_LINE_HIT_PX = 12

  type SeriesStatMark = {
    kind: 'min' | 'max' | 'avg' | 'slope'
    statLabel: string
    valueText: string
    title: string
    color: string
    /** Dot anchor on the chart (extremum x for min/max; selection x for avg/slope). */
    dotDate: Date
    /** Rule anchor y in data space (rate at selection for slope). */
    y: number
    /** Tangent slope in rate-units per second² (rate view only). */
    slope?: number
    /** Bucket endpoints for slope segment (rate view only). */
    slopeFrom?: ChartPoint
    slopeTo?: ChartPoint
    /** Whether to draw a dedicated vertical rule (min/max extremum x). */
    showVertical: boolean
  }

  /** Min / max / avg (or slope segment in rate view) for the selected series. */
  let seriesStatMarks = $derived.by((): SeriesStatMark[] => {
    if (!showStatOverlays) return []
    if (highlightDate === null) return []
    if (!selectedSeriesKey || isAggregateKey(selectedSeriesKey)) return []

    const series = chartSeries.find(s => s.key === selectedSeriesKey)
    if (!series || series.data.length === 0) return []

    const stats = seriesStatsFromPoints(series.data)
    const color = series.color
    const marks: SeriesStatMark[] = []

    const minPoint = extremumPoint(series.data, 'min')
    if (stats.min !== undefined && minPoint !== undefined) {
      const valueText = formatMetricValue(stats.min)
      marks.push({
        kind: 'min',
        statLabel: SERIES_STAT_LABEL.min,
        valueText,
        title: `min ${valueText}`,
        color,
        dotDate: minPoint.date,
        y: stats.min,
        showVertical: true,
      })
    }

    const maxPoint = extremumPoint(series.data, 'max')
    if (
      stats.max !== undefined &&
      maxPoint !== undefined &&
      stats.max !== stats.min
    ) {
      const valueText = formatMetricValue(stats.max)
      marks.push({
        kind: 'max',
        statLabel: SERIES_STAT_LABEL.max,
        valueText,
        title: `max ${valueText}`,
        color,
        dotDate: maxPoint.date,
        y: stats.max,
        showVertical: true,
      })
    }

    if (aggregationView === 'rate') {
      const segment = rateSlopeBucketSegment(series.data, highlightDate)
      if (segment && Number.isFinite(segment.slope)) {
        const valueText = formatRateSlopeValue(segment.slope, unit)
        marks.push({
          kind: 'slope',
          statLabel: SERIES_STAT_LABEL.slope,
          valueText,
          title: `rate slope ${valueText}`,
          color,
          dotDate: segment.to.date,
          y: segment.to.value,
          slope: segment.slope,
          slopeFrom: segment.from,
          slopeTo: segment.to,
          showVertical: false,
        })
      }
    } else if (stats.avg !== undefined && series.data.length > 1) {
      const valueText = formatMetricValue(stats.avg)
      marks.push({
        kind: 'avg',
        statLabel: SERIES_STAT_LABEL.avg,
        valueText,
        title: `avg ${valueText}`,
        color,
        dotDate: highlightDate,
        y: stats.avg,
        showVertical: false,
      })
    }

    return marks
  })

  /** Horizontal guide at the selected series value on the selection x. */
  let selectedPointValueY = $derived.by((): number | null => {
    if (!showStatOverlays || highlightDate === null) return null
    if (!selectedSeriesKey || isAggregateKey(selectedSeriesKey)) return null
    const series = chartSeries.find(s => s.key === selectedSeriesKey)
    if (!series) return null
    const value = nearestValueAt(series.data, highlightDate)
    if (value === undefined || !Number.isFinite(value)) return null
    if (
      aggregationView === 'rate' &&
      selectedRateSlope !== undefined &&
      Number.isFinite(selectedRateSlope)
    ) {
      return null
    }
    return value
  })

  /** Pixel positions for pinned stat labels at each mark's dot. Min below,
   *  max above; avg/slope above/below by min proximity. */
  let seriesStatTooltipPlacements = $derived.by(() => {
    const ctx = lineChartContext
    if (!ctx || seriesStatMarks.length === 0) return []

    const plotLeft = ctx.padding.left
    const plotTop = ctx.padding.top
    const minY = seriesStatMarks.find(m => m.kind === 'min')?.y
    const maxY = seriesStatMarks.find(m => m.kind === 'max')?.y

    return seriesStatMarks.map(mark => {
      const left = ctx.xScale(mark.dotDate) + plotLeft
      const top = ctx.yScale(mark.y) + plotTop

      let placement: 'above' | 'below'
      if (mark.kind === 'min') {
        placement = 'below'
      } else if (mark.kind === 'max') {
        placement = 'above'
      } else if (mark.kind === 'avg' || mark.kind === 'slope') {
        if (minY !== undefined && maxY !== undefined) {
          placement =
            Math.abs(mark.y - minY) <= Math.abs(maxY - mark.y) ? 'above' : 'below'
        } else if (minY !== undefined) {
          placement = 'above'
        } else {
          placement = 'below'
        }
      } else if (minY !== undefined && maxY !== undefined) {
        placement =
          Math.abs(mark.y - minY) <= Math.abs(maxY - mark.y) ? 'above' : 'below'
      } else if (minY !== undefined) {
        placement = 'above'
      } else {
        placement = 'below'
      }

      return { ...mark, placement, left, top }
    })
  })

  let selectionDots = $derived.by((): SelectionDot[] => {
    if (highlightDate === null) return []
    const dots: SelectionDot[] = []
    for (const s of chartSeries) {
      const isSelected = s.key === selectedSeriesKey
      const isAggregate = isAggregateKey(s.key)
      if (!isSelected && !isAggregate) continue
      const v = nearestValueAt(s.data, highlightDate)
      if (v === undefined || !Number.isFinite(v)) continue
      dots.push({ key: s.key, color: s.color, value: v, isSelected })
    }
    return dots
  })

  /** Pre-formatted timestamp string for the legend card header. Reuses
   *  the project-wide `formatDateTime` helper (millisecond resolution,
   *  timezone-aware) so the legend, hover tooltip header, and the
   *  datapoints list all read identical timestamps. */
  let selectionTimestampText = $derived.by((): string => {
    if (highlightDate === null) return ''
    return formatDateTime(
      highlightDate.getTime(),
      timeContext.tz,
      'milliseconds'
    )
  })

  /** Mini-legend: selected series row plus a single aggregate summary line. */
  let selectionLegendRows = $derived.by((): SelectionLegendRow[] => {
    if (selectionDots.length === 0) return []
    const seriesDots = selectionDots.filter((d) => !isAggregateKey(d.key))
    const labelByKey = new Map(chartSeries.map(s => [s.key, s.label] as const))
    return seriesDots.map((d): SelectionLegendRow => ({
      key: d.key,
      color: d.color,
      label: labelByKey.get(d.key) ?? d.key,
      glyph: null,
      glyphTitle: null,
      valueText: formatMetricValue(d.value),
      isPrimary: d.isSelected,
    }))
  })

  let selectionAggregateRows = $derived.by((): AggregateSummaryRow[] => {
    if (highlightDate === null) return []
    return aggregateSummaryRowsAt(selectionDots.map((d) => d.key), (key) => {
      const dot = selectionDots.find((d) => d.key === key)
      return dot?.value
    })
  })
</script>

{#if visiblePointCount > 0}
  <div class="metric-time-series-chart" style:height="{height}px">
    {#if timeRange || selectionLegendRows.length > 0 || selectionAggregateRows.length > 0}
      <div class="metric-time-series-chart__header">
        {#if timeRange}
          <ChartTimeRangeHeader
            startMs={timeRange.startMs}
            endMs={timeRange.endMs}
            variant="legend"
          />
        {/if}
        {#if selectionLegendRows.length > 0 || selectionAggregateRows.length > 0}
          <div class="metric-time-series-chart__selection-legend">
            <ChartSelectionLegend
              timestamp={selectionTimestampText}
              rows={selectionLegendRows}
              aggregateRows={selectionAggregateRows}
            />
          </div>
        {/if}
      </div>
    {/if}
    <div
      class="metric-time-series-chart__plot"
      bind:clientHeight={plotAreaHeight}
    >
    <MetricChartPlot height={plotAreaHeight > 0 ? plotAreaHeight : height}>
      <LineChart
        bind:context={lineChartContext}
        x="date"
        y="value"
        xScale={scaleTime()}
        yNice
        padding={chartPadding}
        tooltipContext={{ mode: 'bisect-x' }}
        series={chartSeries}
        onPointClick={handlePointClick}
        onTooltipClick={handleTooltipClick}
        props={{
          xAxis: axisTime(timeContext.tz),
          yAxis: axisValue(yAxisLabel),
        }}
      >
        {#snippet tooltip({ context }: { context: any })}
          {@const xDate = tooltipXDate(context)}
          {@const rawItems = chartSeries
            .filter(s => !isAggregateKey(s.key))
            .slice()
            .sort((a, b) =>
              String(a.label ?? a.key).localeCompare(String(b.label ?? b.key))
            )}
          {@const aggItems = chartSeries
            .filter(s => isAggregateKey(s.key))
            .slice()
            .sort(
              (a, b) =>
                (AGG_TOTAL_ORDER[a.key] ?? 99) - (AGG_TOTAL_ORDER[b.key] ?? 99)
            )}
          {@const headerLabel =
            xDate != null
              ? formatDateTime(xDate.getTime(), timeContext.tz, 'milliseconds')
              : undefined}
          <Tooltip.Root {context}>
            {#snippet children()}
              <Tooltip.Header value={headerLabel} />
              <Tooltip.List>
                {#each rawItems as s (s.key)}
                  {@const value =
                    xDate != null ? nearestValueAt(s.data, xDate) : undefined}
                  {#if value !== undefined}
                    <Tooltip.Item
                      label={s.label}
                      {value}
                      color={s.color}
                      format={formatMetricValue}
                      valueAlign="right"
                    />
                  {/if}
                {/each}
                {#if aggItems.length > 0 && xDate != null}
                  {@const aggregateRows = aggregateSummaryRowsAt(
                    aggItems.map((s) => s.key),
                    (key) => {
                      const series = aggItems.find((s) => s.key === key)
                      return series ? nearestValueAt(series.data, xDate) : undefined
                    }
                  )}
                  {#if aggregateRows.length > 0}
                    <Tooltip.Separator />
                    <ChartAggregateSummaryRows
                      rows={aggregateRows}
                      class="lc-tooltip-agg-summary"
                    />
                  {/if}
                {/if}
              </Tooltip.List>
            {/snippet}
          </Tooltip.Root>
        {/snippet}

        {#snippet aboveMarks({ context }: { context: any })}
          {@const xLeft = context.xRange[0]}
          {@const xRight = context.xRange[1]}
          {@const yTop = context.yRange[1]}
          {@const yBot = context.yRange[0]}
          {#if seriesStatMarks.length > 0 || selectedPointValueY !== null}
            <g
              class="series-stat-overlay"
              style:--marker-color={seriesStatMarks[0]?.color ??
                chartSeries.find(s => s.key === selectedSeriesKey)?.color}
            >
              {#if selectedPointValueY !== null}
                {@const yPx = context.yScale(selectedPointValueY)}
                {@const hitHalf = STAT_LINE_HIT_PX / 2}
                <g class="series-stat-line-group">
                  <rect
                    class="series-stat-hitbox"
                    x={xLeft}
                    y={yPx - hitHalf}
                    width={xRight - xLeft}
                    height={STAT_LINE_HIT_PX}
                  />
                  <Line
                    x1={xLeft}
                    x2={xRight}
                    y1={yPx}
                    y2={yPx}
                    class="series-stat-line series-stat-line--point-value"
                  />
                </g>
              {/if}
              {#each seriesStatMarks as mark (mark.kind)}
                {@const dotPx = context.xScale(mark.dotDate)}
                {@const dotYPx = context.yScale(mark.y)}
                {@const vPx = context.xScale(mark.dotDate)}
                {@const plotBandTop = Math.min(yTop, yBot)}
                {@const plotBandHeight = Math.abs(yBot - yTop)}
                {@const hitHalf = STAT_LINE_HIT_PX / 2}
                <g class="series-stat-marker">
                  <title>{mark.title}</title>
                  {#if mark.kind === 'slope' && mark.slopeFrom && mark.slopeTo}
                    {@const segX1 = context.xScale(mark.slopeFrom.date)}
                    {@const segY1 = context.yScale(mark.slopeFrom.value)}
                    {@const segX2 = context.xScale(mark.slopeTo.date)}
                    {@const segY2 = context.yScale(mark.slopeTo.value)}
                    <g class="series-stat-line-group">
                      <Line
                        x1={segX1}
                        y1={segY1}
                        x2={segX2}
                        y2={segY2}
                        class="series-stat-hitbox series-stat-hitbox--line"
                      />
                      <Line
                        x1={segX1}
                        y1={segY1}
                        x2={segX2}
                        y2={segY2}
                        class="series-stat-line"
                        markerEnd="arrow"
                      />
                    </g>
                  {:else}
                    {@const yPx = dotYPx}
                    <g class="series-stat-line-group">
                      <rect
                        class="series-stat-hitbox"
                        x={xLeft}
                        y={yPx - hitHalf}
                        width={xRight - xLeft}
                        height={STAT_LINE_HIT_PX}
                      />
                      <Line
                        x1={xLeft}
                        x2={xRight}
                        y1={yPx}
                        y2={yPx}
                        class="series-stat-line"
                      />
                    </g>
                    {#if mark.showVertical}
                      <g class="series-stat-line-group">
                        <rect
                          class="series-stat-hitbox"
                          x={vPx - hitHalf}
                          y={plotBandTop}
                          width={STAT_LINE_HIT_PX}
                          height={plotBandHeight}
                        />
                        <Line
                          x1={vPx}
                          x2={vPx}
                          y1={yTop}
                          y2={yBot}
                          class="series-stat-line"
                        />
                      </g>
                    {/if}
                  {/if}
                  <circle
                    cx={dotPx}
                    cy={dotYPx}
                    r="4"
                    fill={mark.color}
                    class="series-stat-dot"
                  />
                </g>
              {/each}
              {#if highlightDate}
                {@const px = context.xScale(highlightDate)}
                <g class="selection-overlay" aria-hidden="true">
                  <Line
                    x1={px}
                    x2={px}
                    y1={yTop}
                    y2={yBot}
                    class="highlight-rule"
                  />
                  {#each selectionDots as dot (dot.key)}
                    {@const py = context.yScale(dot.value)}
                    <!-- Halo ring drawn first so the colored dot sits on
                         top. Stroke-only so the line's own color shows
                         through the center, keeping the dot readable
                         against overlapping series. -->
                    <circle
                      cx={px}
                      cy={py}
                      r="8"
                      class="selection-dot-halo"
                      class:selection-dot-halo--selected={dot.isSelected}
                    />
                    <circle
                      cx={px}
                      cy={py}
                      r="6"
                      fill={dot.color}
                      class="selection-dot"
                      class:selection-dot--selected={dot.isSelected}
                    />
                  {/each}
                </g>
              {/if}
            </g>
          {:else if highlightDate}
            {@const px = context.xScale(highlightDate)}
            <g class="selection-overlay" aria-hidden="true">
              <Line
                x1={px}
                x2={px}
                y1={yTop}
                y2={yBot}
                class="highlight-rule"
              />
              {#each selectionDots as dot (dot.key)}
                {@const py = context.yScale(dot.value)}
                <circle
                  cx={px}
                  cy={py}
                  r="8"
                  class="selection-dot-halo"
                  class:selection-dot-halo--selected={dot.isSelected}
                />
                <circle
                  cx={px}
                  cy={py}
                  r="6"
                  fill={dot.color}
                  class="selection-dot"
                  class:selection-dot--selected={dot.isSelected}
                />
              {/each}
            </g>
          {/if}
        {/snippet}
      </LineChart>
      {#each seriesStatTooltipPlacements as mark (mark.kind)}
        <div
          class="series-stat-tooltip"
          class:series-stat-tooltip--above={mark.placement === 'above'}
          class:series-stat-tooltip--below={mark.placement === 'below'}
          style:left="{mark.left}px"
          style:top="{mark.top}px"
          title={mark.title}
          aria-hidden="true"
        >
          <div class="chart-selection-legend chart-selection-legend--stat">
            <ul class="chart-selection-legend__rows">
              <li class="chart-selection-legend__row">
                <span
                  class="chart-selection-legend__dot"
                  style:--color={mark.color}
                  aria-hidden="true"
                ></span>
                <span class="chart-selection-legend__label"
                  >{mark.statLabel}</span
                >
                <span class="chart-selection-legend__value"
                  >{mark.valueText}</span
                >
              </li>
            </ul>
          </div>
        </div>
      {/each}
    </MetricChartPlot>
    </div>
  </div>
{:else}
  <MetricChartEmpty {height} message={emptyMessage} />
{/if}

<style lang="postcss">
  @reference "../../../app.css";

  .metric-time-series-chart {
    @apply flex min-h-0 w-full min-w-0 flex-col;
  }

  .metric-time-series-chart__header {
    @apply flex shrink-0 items-start justify-between gap-2 px-1 pb-1 pt-0.5;
  }

  .metric-time-series-chart__plot {
    @apply relative min-h-0 min-w-0 flex-1;
  }

  .metric-time-series-chart__selection-legend {
    @apply ml-auto shrink-0;
    pointer-events: none;
  }

  /* Pinned min/max/avg labels: selection-legend card at each stat dot. */
  .series-stat-tooltip {
    position: absolute;
    pointer-events: none;
    z-index: 2;
    width: max-content;
    max-width: none;
  }

  .series-stat-tooltip--above {
    transform: translate(-50%, calc(-100% - 8px));
  }

  .series-stat-tooltip--below {
    transform: translate(-50%, 8px);
  }

  /* Aggregate summary rows span the tooltip grid when embedded. */
  :global(.lc-tooltip-agg-summary) {
    grid-column: 1 / -1;
  }

  /* Selection rule + dots must not steal clicks from the chart surface. */
  :global(.selection-overlay) {
    pointer-events: none;
  }
</style>

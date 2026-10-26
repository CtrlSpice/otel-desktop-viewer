<script lang="ts">
  import ChartOverlayLabel from './ChartOverlayLabel.svelte'
  import { createOverlayEmphasis } from '@/components/metrics/utils/overlay-emphasis.svelte'

  import ChartSticky from '@/components/metrics/Charts/ChartSticky.svelte'
  import {
    LineChart,
    Line,
    Rule,
    Spline,
    Tooltip,
    type ChartState,
  } from 'layerchart'
  import { bisector } from 'd3-array'
  import { scaleTime } from 'd3-scale'
  import { curveStepAfter } from 'd3-shape'
  import {
    formatMetricValue,
    formatMetricHoverValue,
    formatRateSlopeValue,
  } from '@/components/metrics/utils/format-metric-value'
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import { formatDateTime } from '@/utils/time'
  import ChartKeyboardSurface from '@/components/metrics/Charts/ChartKeyboardSurface.svelte'
  import MetricChartEmpty from '@/components/metrics/Charts/MetricChartEmpty.svelte'
  import MetricChartPlot, {
    axisTime,
    axisValue,
    timeSeriesChartPadding as fallbackPadding,
    DEFAULT_METRIC_CHART_HEIGHT,
  } from '@/components/metrics/Charts/MetricChartPlot.svelte'
  import { chartNeutral } from '@/utils/chart-palette'
  import { extendChartLine } from '@/components/metrics/utils/extend-chart-line'
  import ChartAggregateSummaryRows from '@/components/metrics/Charts/ChartAggregateSummaryRows.svelte'
  import ChartSelectionLegend, {
    type SelectionLegendRow,
  } from '@/components/metrics/Charts/ChartSelectionLegend.svelte'
  import {
    buildAggregateSummaryRows,
    type AggregateLineKey,
    type AggregateSummaryRow,
    type AggregationView,
    isAggregateLineKey,
    rateSlopeViewSymbol,
    rateSlopeBucketSegment,
    type SeriesStats,
  } from '@/components/metrics/utils/aggregation'
  import type {
    ChartPoint,
    ChartTimeseries,
    LayerChartPointClickDetail,
    LayerChartSeriesDatum,
    LayerChartTooltipClickDetail,
  } from '@/types/metric-chart-types'
  import {
    isChartActivationKey,
    lineCursorAt,
    lineCursorCommandForKey,
    stablePointCursorKey,
  } from '@/components/metrics/utils/chart-keyboard-cursor'
  import { createLineChartKeyboardCursor } from '@/components/metrics/utils/chart-keyboard-state.svelte'
  import { timeSeriesChartPointSelection } from '@/components/metrics/utils/time-series-point-selection'
  import { measureChartAxisGutter } from '@/components/metrics/utils/chart-axis-gutter'

  let yAxisInset = $state<number>(fallbackPadding.left)
  const chartPadding = $derived({ ...fallbackPadding, left: yAxisInset })

  const emphasis = createOverlayEmphasis()

  function aggregateSummaryRowsAt(
    keys: readonly string[],
    valueAt: (key: string) => number | undefined
  ): AggregateSummaryRow[] {
    return buildAggregateSummaryRows(
      aggregateKeysInOrder(keys),
      aggregationView,
      key => valueAt(key),
      formatMetricHoverValue
    )
  }

  function aggregateKeysInOrder(keys: readonly string[]): AggregateLineKey[] {
    return keys.filter(isAggregateLineKey)
  }

  /** Nearest point at `x` — layerchart's default tooltip matches exact
   *  timestamps only, which breaks when raw and aggregate grids differ. */
  function nearestValueAt(
    points: readonly { date: Date; value: number }[],
    x: Date
  ): number | undefined {
    return nearestPointAt(points, x)?.value
  }

  function nearestPointAt<T extends { date: Date }>(
    points: readonly T[],
    x: Date
  ): T | undefined {
    if (points.length === 0) return undefined
    const t = x.getTime()
    const i = bisector((p: { date: Date }) => p.date.getTime()).left(points, t)
    const lo = Math.max(0, i - 1)
    const hi = Math.min(points.length - 1, i)
    const dl = Math.abs(points[lo]!.date.getTime() - t)
    const dh = Math.abs(points[hi]!.date.getTime() - t)
    return dl <= dh ? points[lo] : points[hi]
  }

  function tooltipXDate(context: {
    tooltip: { data: LayerChartSeriesDatum<ChartPoint> | null }
    x: (
      d: LayerChartSeriesDatum<ChartPoint>
    ) => Date | string | number | null | undefined
    valueAxis: string
  }): Date | null {
    const d = context.tooltip.data
    if (d == null) return null
    const v = context.x(d)
    if (v instanceof Date) return v
    if (v != null) return new Date(v)
    return null
  }

  type Props = {
    /** Pre-grouped, already visibility-filtered timeseries. The caller
     *  (MetricViewContext via MetricChartView) drops timeseries the
     *  user has unchecked in the legend BEFORE handing the array down,
     *  so the chart trusts every entry here is meant to render. */
    timeseries: ChartTimeseries[]
    height?: number
    /** Exact timestamp of the displayed selection. This is a source timestamp
     * for ordinary lines and the containing synthetic bucket in Rate view. */
    highlightedTimestamp?: bigint | null
    /** Exact source id for ordinary raw lines. Rate points are synthetic and
     * resolve by `highlightedTimestamp` instead. */
    highlightedPointID?: string | null
    /** Attributes key of the timeseries owning the selected datapoint.
     *  When set alongside `highlightedTimestamp`, the chart draws a
     *  colored dot on the rule at that series's value. Aggregates
     *  always get dots too -- the selected series is the one we'd
     *  otherwise not be able to identify just from x-position. */
    selectedSeriesKey?: string | null
    /** Transient emphasis from a visible series header. */
    previewSeriesKey?: string | null
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
    /** Plotted data window; rendered as a permanent legend card above the plot. */
    timeRange?: { startMs: number; endMs: number } | null
    /** Source point click -> exact datapoint selection. Synthetic points omit
     *  an id and remain inspectable but read-only. */
    onChartPointClick?: (seriesKey: string, datapointID: string) => void
    onRateChartPointClick?: (seriesKey: string, timestampNs: bigint) => void
    onClearSelection?: () => void
    /** Store-computed selected-series stats for the source window or rate line. */
    seriesStats?: ReadonlyMap<string, SeriesStats>
  }

  let {
    timeseries,
    height = DEFAULT_METRIC_CHART_HEIGHT,
    highlightedTimestamp = null,
    highlightedPointID = null,
    selectedSeriesKey = null,
    previewSeriesKey = null,
    unit = '',
    colorByKey,
    emptyMessage = 'No datapoints to chart',
    aggregationView = 'raw',
    showStatOverlays = true,
    timeRange = null,
    onChartPointClick,
    onRateChartPointClick,
    onClearSelection,
    seriesStats,
  }: Props = $props()

  const timeContext = getTimeContext()

  /* One scale template per chart; LineChart copies it before use. */
  const xScale = scaleTime()

  /* Stable identity avoids invalidating the tooltip context. */
  const tooltipContext = { mode: 'bisect-x' } as const

  // Build the layerchart series array on the fly. Each entry carries
  // its own pre-grouped data so we don't re-traverse on every chart
  // re-render. Colour is looked up via the caller-provided `colorByKey`
  // map (keyed by seriesRef), not by position in this prop's array --
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

  function seriesLineProps(key: string) {
    if (isAggregateLineKey(key)) return { props: AGG_LINE_PROPS }
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

  let keyboardLines = $derived(
    chartSeries
      .filter(series => series.data.length > 0)
      .map(series => ({
        key: series.key,
        points: series.data.map((point, pointIndex) => ({
          key: stablePointCursorKey(
            {
              sourceKey: point.sourceDatapointID,
              timestampNs: point.timestampNs,
              timestampMs: point.date.getTime(),
            },
            pointIndex
          ),
          timestampMs: point.date.getTime(),
          timestampNs: point.timestampNs,
        })),
      }))
  )
  let initialKeyboardCursor = $derived.by(() => {
    const sourcePointID = aggregationView === 'rate' ? null : highlightedPointID
    if (sourcePointID === null && highlightedTimestamp === null) {
      return null
    }
    const timestampMs =
      highlightedTimestamp == null
        ? null
        : Number(highlightedTimestamp / 1_000_000n)
    return lineCursorAt(keyboardLines, {
      lineKey: selectedSeriesKey,
      pointKey: sourcePointID
        ? stablePointCursorKey(
            {
              sourceKey: sourcePointID,
              timestampMs: timestampMs ?? 0,
            },
            0
          )
        : null,
      timestampMs,
      timestampNs: highlightedTimestamp,
      requireExact: true,
    })
  })
  const keyboardNavigation = createLineChartKeyboardCursor(
    () => keyboardLines,
    () => initialKeyboardCursor
  )
  let activeKeyboardCursor = $derived(keyboardNavigation.current)
  let keyboardFocused = $derived(keyboardNavigation.focused)

  function handleKeyboardFocusChange(focused: boolean) {
    keyboardNavigation.setFocused(focused)
  }

  let keyboardSeries = $derived.by(() => {
    if (!activeKeyboardCursor) return undefined
    return chartSeries.find(
      series => series.key === activeKeyboardCursor.lineKey
    )
  })
  let keyboardPoint = $derived.by(() => {
    const cursor = activeKeyboardCursor
    const series = keyboardSeries
    if (!cursor || !series) return undefined
    return series.data[cursor.pointIndex]
  })

  function keyboardValueText(value: number): string {
    const formatted = formatMetricValue(value)
    const trimmedUnit = unit.trim()
    if (aggregationView === 'rate') {
      return trimmedUnit && trimmedUnit !== '1'
        ? `${formatted} ${trimmedUnit}/s`
        : `${formatted}/s`
    }
    return trimmedUnit && trimmedUnit !== '1'
      ? `${formatted} ${trimmedUnit}`
      : formatted
  }

  let keyboardReadout = $derived.by((): string => {
    const cursor = activeKeyboardCursor
    const series = keyboardSeries
    const point = keyboardPoint
    if (!cursor || !series || !point) return ''
    const timestamp = formatDateTime(
      point.date.getTime(),
      timeContext.tz,
      'milliseconds'
    )
    const rateSelectable =
      aggregationView === 'rate' &&
      onRateChartPointClick &&
      point.timestampNs !== undefined &&
      !isAggregateLineKey(series.key)
    const readOnly =
      point.sourceDatapointID || rateSelectable
        ? ''
        : ' Read-only synthetic point.'
    return (
      `Line ${cursor.lineIndex + 1} of ${keyboardLines.length}, ${series.label}. ` +
      `Point ${cursor.pointIndex + 1} of ${series.data.length}, ${timestamp}. ` +
      `Value ${keyboardValueText(point.value)}.${readOnly}`
    )
  })

  function handleChartKeyboard(event: KeyboardEvent) {
    const command = lineCursorCommandForKey(event)
    if (command) {
      event.preventDefault()
      event.stopPropagation()
      keyboardNavigation.move(command)
      return
    }

    if (isChartActivationKey(event.key)) {
      const point = keyboardPoint
      const key = activeKeyboardCursor?.lineKey
      if (!point || !key || !activateChartPoint(key, point)) return
      event.preventDefault()
      event.stopPropagation()
      return
    }

    if (event.key === 'Escape') {
      event.preventDefault()
      event.stopPropagation()
      onClearSelection?.()
    }
  }

  let keyboardCursorMark = $derived.by(() => {
    if (!keyboardFocused) return null
    const cursor = activeKeyboardCursor
    const series = keyboardSeries
    const point = keyboardPoint
    if (!cursor || !series || !point) return null
    return { date: point.date, value: point.value, color: series.color }
  })
  let keyboardCanActivate = $derived(
    chartSeries.some(
      series =>
        !isAggregateLineKey(series.key) &&
        series.data.some(
          point =>
            (!!onChartPointClick && point.sourceDatapointID !== undefined) ||
            (aggregationView === 'rate' &&
              !!onRateChartPointClick &&
              point.timestampNs !== undefined)
        )
    )
  )
  let keyboardShortcuts = $derived([
    'ArrowLeft',
    'ArrowRight',
    'ArrowUp',
    'ArrowDown',
    'Home',
    'End',
    'Control+Home',
    'Control+End',
    'Meta+Home',
    'Meta+End',
    ...(keyboardCanActivate ? ['Enter', 'Space'] : []),
    'Escape',
  ])
  let keyboardInstructions = $derived(
    `Use Left and Right to inspect points. Use Up and Down to inspect lines. Home and End move to line boundaries; Control or Command plus Home or End move to chart boundaries.${keyboardCanActivate ? ' Enter or Space selects the current datapoint or Rate bucket.' : ''} Escape clears chart selection.`
  )

  let lineChartContext = $state<ChartState<ChartPoint> | undefined>(undefined)

  // Resolve one exact sample for emphasis without rebuilding series data or
  // moving a point onto another series' timestamp grid.
  let hoveredPoint = $derived.by(() => {
    const context = lineChartContext
    if (!context?.tooltip.isHoveringTooltipArea) return null
    const date = tooltipXDate(context)
    if (!date) return null
    const x = context.tooltip.x - context.padding.left
    const y = context.tooltip.y - context.padding.top
    let closest: {
      key: string
      color: string
      point: ChartPoint
      distance: number
    } | null = null
    for (const series of chartSeries) {
      const point = nearestPointAt(series.data, date)
      if (!point) continue
      const distance = Math.hypot(
        context.xScale(point.date) - x,
        context.yScale(point.value) - y
      )
      if (!closest || distance < closest.distance) {
        closest = { key: series.key, color: series.color, point, distance }
      }
    }
    return closest
  })

  let emphasizedSeriesKey = $derived(
    chartSeries.some(series => series.key === previewSeriesKey)
      ? previewSeriesKey
      : keyboardFocused
        ? keyboardSeries?.key
        : hoveredPoint?.key
  )
  /** The plot uses the full chart height; pinned cards do not reserve a row. */
  let plotAreaHeight = $state(0)

  /** Plot y-value at the pointer, for disambiguating series at a shared x. */
  function pointerYDataValue(e: MouseEvent): number | undefined {
    const ctx = lineChartContext
    if (!ctx?.yScale?.invert) return undefined
    const target = e.target
    if (!(target instanceof Element)) return undefined
    const root = target.closest('.lc-root-container')
    if (!root) return undefined
    const rect = root.getBoundingClientRect()
    const plotY = e.clientY - rect.top - ctx.padding.top
    const value = ctx.yScale.invert(plotY)
    return Number.isFinite(value) ? value : undefined
  }

  /** Pick the raw series whose value at `date` is closest to click y. */
  function seriesKeyAtPointerY(e: MouseEvent, date: Date): string | null {
    const lineKey =
      e.target instanceof Element
        ? e.target.closest<SVGPathElement>('.metric-series-line')?.dataset
            .seriesKey
        : undefined
    if (lineKey) return lineKey
    const clickY = pointerYDataValue(e)
    if (clickY === undefined) return null

    let bestKey: string | null = null
    let bestDist = Infinity
    for (const s of timeseries) {
      if (isAggregateLineKey(s.key)) continue
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

  /** Map a chart row back to a raw series key. With multiple series at the
   *  same x, match on y/value before falling back to per-series lookup. */
  function seriesKeyForChartPoint(
    data: LayerChartSeriesDatum<ChartPoint>,
    explicitKey?: string | null
  ): string | null {
    if (explicitKey && !isAggregateLineKey(explicitKey)) {
      return explicitKey
    }

    const date = data.date
    const t = date.getTime()
    const clickedValue = data.value

    if (Number.isFinite(clickedValue)) {
      for (const s of timeseries) {
        if (isAggregateLineKey(s.key)) continue
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
        if (isAggregateLineKey(s.key)) continue
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
        if (isAggregateLineKey(s.key)) continue
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
      if (isAggregateLineKey(s.key)) continue
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

  function sourcePointForChartPoint(
    seriesKey: string,
    data: LayerChartSeriesDatum<ChartPoint>,
    date: Date
  ): ChartPoint | null {
    const series = chartSeries.find(candidate => candidate.key === seriesKey)
    if (!series) return null

    const sourceDatapointID = data.sourceDatapointID
    if (sourceDatapointID) {
      return (
        series.data.find(
          point => point.sourceDatapointID === sourceDatapointID
        ) ?? null
      )
    }

    const timestampNs = data.timestampNs
    if (timestampNs !== undefined) {
      return (
        series.data.find(point => point.timestampNs === timestampNs) ?? null
      )
    }

    const sameMillisecond = series.data.filter(
      point => point.date.getTime() === date.getTime()
    )
    if (sameMillisecond.length === 1) return sameMillisecond[0]!
    const sameValue = sameMillisecond.filter(
      point => point.value === data.value
    )
    return sameValue.length === 1 ? sameValue[0]! : null
  }

  function dispatchTooltipClick(
    e: MouseEvent,
    data: LayerChartSeriesDatum<ChartPoint>
  ) {
    if (!onChartPointClick && !onRateChartPointClick) return
    const date = data.date
    let key = seriesKeyAtPointerY(e, date)
    if (!key) {
      key = seriesKeyForChartPoint(data)
    }
    if (!key || isAggregateLineKey(key)) return
    const point = sourcePointForChartPoint(key, data, date)
    if (point && activateChartPoint(key, point)) e.stopPropagation()
  }

  function activateChartPoint(seriesKey: string, point: ChartPoint): boolean {
    if (isAggregateLineKey(seriesKey)) return false
    if (
      aggregationView === 'rate' &&
      onRateChartPointClick &&
      point.timestampNs !== undefined
    ) {
      onRateChartPointClick(seriesKey, point.timestampNs)
      return true
    }
    if (point.sourceDatapointID && onChartPointClick) {
      onChartPointClick(seriesKey, point.sourceDatapointID)
      return true
    }
    return false
  }

  function handlePointClick(
    e: MouseEvent,
    details: LayerChartPointClickDetail<ChartPoint>
  ) {
    if (details.point?.seriesKey === undefined) return
    if (aggregationView === 'rate') {
      const key = details.point.seriesKey
      const timestampNs = details.data.timestampNs
      if (timestampNs === undefined) return
      const point = chartSeries
        .find(series => series.key === key)
        ?.data.find(point => point.timestampNs === timestampNs)
      if (point && activateChartPoint(key, point)) e.stopPropagation()
      return
    }
    const selection = timeSeriesChartPointSelection(details, timeseries)
    if (selection === null || isAggregateLineKey(selection.seriesKey)) return
    e.stopPropagation()
    onChartPointClick?.(selection.seriesKey, selection.sourceDatapointID)
  }

  /** Bare plot/line clicks can lack LayerChart's tooltip datum. Snap using the
   * drawn points, then activate their real source id or exact Rate bucket. */
  function handlePlotClick(event: MouseEvent) {
    const context = lineChartContext
    const target = event.target
    if (!context || !(target instanceof Element)) return
    const lineKey = target.closest<SVGPathElement>('.metric-series-line')
      ?.dataset.seriesKey
    if (lineKey && isAggregateLineKey(lineKey)) {
      emphasis.toggle(lineKey)
      event.stopPropagation()
      return
    }
    const root = target.closest('.lc-root-container')
    if (!root) return
    const bounds = root.getBoundingClientRect()
    const x = event.clientX - bounds.left - context.padding.left
    const y = event.clientY - bounds.top - context.padding.top
    const xMin = Math.min(...context.xRange)
    const xMax = Math.max(...context.xRange)
    const yMin = Math.min(...context.yRange)
    const yMax = Math.max(...context.yRange)
    if (x < xMin || x > xMax || y < yMin || y > yMax) return
    let nearest: {
      seriesKey: string
      point: ChartPoint
      distance: number
    } | null = null
    for (const series of chartSeries) {
      if (isAggregateLineKey(series.key)) continue
      if (lineKey && series.key !== lineKey) continue
      for (const point of series.data) {
        const selectable =
          aggregationView === 'rate'
            ? !!onRateChartPointClick && point.timestampNs !== undefined
            : !!onChartPointClick && point.sourceDatapointID !== undefined
        if (!selectable) continue
        const distance = Math.hypot(
          context.xScale(point.date) - x,
          context.yScale(point.value) - y
        )
        if (!nearest || distance < nearest.distance)
          nearest = { seriesKey: series.key, point, distance }
      }
    }
    if (nearest && activateChartPoint(nearest.seriesKey, nearest.point))
      event.stopPropagation()
  }

  /** Chart-area click under bisect-x tooltip — disambiguate series by
   *  pointer y, not the bisected row's value (wrong with many series). */
  function handleTooltipClick(
    e: MouseEvent,
    detail: LayerChartTooltipClickDetail<ChartPoint>
  ) {
    dispatchTooltipClick(e, detail.data)
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

  /* Keep responsive ticks, omitting the zero tick and label only where the
   * rounded bottom axis already establishes the baseline. */
  let chartProps = $derived({
    xAxis: axisTime(timeContext.tz),
    yAxis: {
      ...axisValue(yAxisLabel),
      ticks: lineChartContext?.yScale
        .ticks?.(Math.round(lineChartContext.height / 50))
        .filter(
          (value: number) =>
            value !== 0 ||
            lineChartContext!.yScale(0) !==
              Math.max(...lineChartContext!.yRange)
        ),
    },
  })

  /** Series the selection rule should drop colored dots on. Persistent dots
   * use source identity for the selected series and exact nanoseconds for
   * aggregates; a line without that exact point honestly gets no dot. */
  type SelectionDot = {
    key: string
    color: string
    value: number
    isSelected: boolean
  }

  function exactSelectionPoint(series: {
    key: string
    data: readonly ChartPoint[]
  }): ChartPoint | undefined {
    if (
      aggregationView !== 'rate' &&
      series.key === selectedSeriesKey &&
      highlightedPointID !== null &&
      highlightedPointID !== undefined
    ) {
      return series.data.find(
        point => point.sourceDatapointID === highlightedPointID
      )
    }
    if (highlightedTimestamp === null || highlightedTimestamp === undefined) {
      return undefined
    }
    return series.data.find(point => point.timestampNs === highlightedTimestamp)
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

  type SeriesStatMark = {
    kind: 'min' | 'max' | 'avg' | 'slope'
    statLabel: string
    valueText: string
    title: string
    color: string
    /** Time anchor for extrema and the selected slope segment. */
    dotDate: Date
    /** Rule anchor y in data space (rate at selection for slope). */
    y: number
    /** Tangent slope in rate-units per second² (rate view only). */
    slope?: number
    /** Bucket endpoints for slope segment (rate view only). */
    slopeFrom?: ChartPoint
    slopeTo?: ChartPoint
  }

  let statLabelSizes = $state<
    Partial<Record<string, { width: number; height: number }>>
  >({})
  type StatLabelPlacement = {
    kind: string
    left: number
    top: number
    placement: 'right' | 'above' | 'below'
  }
  function statLabelBounds(mark: StatLabelPlacement) {
    const size = statLabelSizes[mark.kind]
    if (!size) return undefined
    // Match the rendered label's CSS translation in chart coordinates.
    return {
      left:
        mark.placement === 'right' ? mark.left + 8 : mark.left - size.width / 2,
      top:
        mark.placement === 'right'
          ? mark.top - size.height / 2
          : mark.placement === 'below'
            ? mark.top + 8
            : mark.top - size.height - 8,
      ...size,
    }
  }
  function measureStatLabel(element: HTMLElement, kind: string) {
    const measure = () => {
      const { width, height } = element.getBoundingClientRect()
      const previous = statLabelSizes[kind]
      if (
        height > 0 &&
        (previous?.width !== width || previous?.height !== height)
      )
        statLabelSizes[kind] = { width, height }
    }
    measure()
    const observer = new ResizeObserver(measure)
    observer.observe(element)
    return { destroy: () => observer.disconnect() }
  }

  /** Use an explicit series selection, or the only visible source series. */
  const statSeries = $derived.by(() => {
    if (selectedSeriesKey)
      return chartSeries.find(series => series.key === selectedSeriesKey)
    const sources = chartSeries.filter(
      series => !isAggregateLineKey(series.key)
    )
    return sources.length === 1 ? sources[0] : undefined
  })

  /** Whole-window stats need a series; only rate slope needs a selected point. */
  let seriesStatMarks = $derived.by((): SeriesStatMark[] => {
    if (!showStatOverlays) return []
    const series = statSeries
    if (!series || isAggregateLineKey(series.key) || series.data.length === 0)
      return []

    const stats = seriesStats?.get(series.key)
    if (!stats) return []
    const color = 'var(--color-primary)'
    const marks: SeriesStatMark[] = []

    const minPoint = extremumPoint(series.data, 'min')
    if (stats.min !== undefined && minPoint !== undefined) {
      const valueText = formatMetricValue(stats.min)
      marks.push({
        kind: 'min',
        statLabel: SERIES_STAT_LABEL.min,
        valueText,
        title: `min ${formatMetricHoverValue(stats.min)}`,
        color: series.color,
        dotDate: minPoint.date,
        y: stats.min,
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
        title: `max ${formatMetricHoverValue(stats.max)}`,
        color: series.color,
        dotDate: maxPoint.date,
        y: stats.max,
      })
    }

    if (aggregationView === 'rate') {
      const selectedPoint = exactSelectionPoint(series)
      const segment = selectedPoint
        ? rateSlopeBucketSegment(series.data, selectedPoint)
        : undefined
      if (segment && Number.isFinite(segment.slope)) {
        const valueText = formatRateSlopeValue(segment.slope, unit)
        marks.push({
          kind: 'slope',
          statLabel: SERIES_STAT_LABEL.slope,
          valueText,
          title: `rate slope ${formatRateSlopeValue(segment.slope, unit, { maxSignificantDigits: 6 })}`,
          color,
          dotDate: segment.to.date,
          y: segment.to.value,
          slope: segment.slope,
          slopeFrom: segment.from,
          slopeTo: segment.to,
        })
      }
    } else if (stats.avg !== undefined && series.data.length > 1) {
      const valueText = formatMetricValue(stats.avg)
      marks.push({
        kind: 'avg',
        statLabel: SERIES_STAT_LABEL.avg,
        valueText,
        title: `avg ${formatMetricHoverValue(stats.avg)}`,
        color,
        dotDate: series.data[0]!.date,
        y: stats.avg,
      })
    }

    return marks
  })

  /** Min/max labels stay on their points; average and slope use the right edge. */
  let seriesStatTooltipPlacements = $derived.by(() => {
    const ctx = lineChartContext
    if (!ctx || seriesStatMarks.length === 0) return []

    const plotLeft = ctx.padding.left
    const plotTop = ctx.padding.top
    return seriesStatMarks.map(mark => {
      const atRight = mark.kind === 'avg' || mark.kind === 'slope'
      const left =
        (atRight ? ctx.xRange[1] : ctx.xScale(mark.dotDate)) + plotLeft
      const top = ctx.yScale(mark.y) + plotTop

      const placement: StatLabelPlacement['placement'] = atRight
        ? 'right'
        : mark.kind === 'min' ||
            top < (statLabelSizes[mark.kind]?.height ?? 0) + 8
          ? 'below'
          : 'above'

      return { ...mark, placement, left, top }
    })
  })

  const aggregateLabelPlacements = $derived.by(() => {
    const ctx = lineChartContext
    if (!ctx) return []
    const labels = chartSeries.flatMap(series => {
      if (!isAggregateLineKey(series.key)) return []
      const point = series.data.at(-1)
      if (!point || !Number.isFinite(point.value)) return []
      return [
        {
          kind: series.key,
          label: series.label,
          color: series.color,
          placement: 'right' as const,
          left: ctx.padding.left + ctx.xScale(point.date),
          top: ctx.padding.top + ctx.yScale(point.value),
          title: `Combined ${series.label.toLowerCase()} across checked series\n${formatMetricHoverValue(point.value)}${unit ? ` ${unit}` : ''}\n${formatDateTime(point.date.getTime(), timeContext.tz, 'milliseconds')}`,
        },
      ]
    })
    const obstacles = seriesStatTooltipPlacements.flatMap(mark => {
      const bounds = statLabelBounds(mark)
      return bounds ? [bounds] : []
    })
    // Use measured labels, including source statistics. Only move vertically
    // near the endpoint; accept overlap when there is no local free position.
    const clearance = 4
    const maxAnchorDistance = 48
    for (const label of labels) {
      const bounds = statLabelBounds(label)
      if (!bounds) continue
      const neighbors = obstacles.filter(
        obstacle =>
          bounds.left < obstacle.left + obstacle.width + clearance &&
          bounds.left + bounds.width + clearance > obstacle.left
      )
      const candidates = [
        label.top,
        ...neighbors.flatMap(obstacle => [
          obstacle.top - clearance - bounds.height / 2,
          obstacle.top + obstacle.height + clearance + bounds.height / 2,
        ]),
      ]
        .filter(
          top =>
            Math.hypot(
              bounds.left - label.left,
              Math.max(Math.abs(top - label.top) - bounds.height / 2, 0)
            ) <= maxAnchorDistance
        )
        .sort((a, b) => Math.abs(a - label.top) - Math.abs(b - label.top))
      const top = candidates.find(center => {
        const upper = center - bounds.height / 2
        const lower = center + bounds.height / 2
        return (
          upper >= 0 &&
          lower <= (plotAreaHeight || height) &&
          neighbors.every(
            obstacle =>
              lower + clearance <= obstacle.top ||
              upper >= obstacle.top + obstacle.height + clearance
          )
        )
      })
      if (top !== undefined) label.top = top
      obstacles.push({ ...bounds, top: label.top - bounds.height / 2 })
    }
    return labels
  })

  let selectionDots = $derived.by((): SelectionDot[] => {
    if (highlightDate === null) return []
    const dots: SelectionDot[] = []
    for (const s of chartSeries) {
      const isSelected = s.key === selectedSeriesKey
      const isAggregate = isAggregateLineKey(s.key)
      if (!isSelected && !isAggregate) continue
      const v = exactSelectionPoint(s)?.value
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
    const seriesDots = selectionDots.filter(d => !isAggregateLineKey(d.key))
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
    return aggregateSummaryRowsAt(
      selectionDots.map(d => d.key),
      key => {
        const dot = selectionDots.find(d => d.key === key)
        return dot?.value
      }
    )
  })

  let selectionAnchor = $derived.by(() => {
    const context = lineChartContext
    const dot = selectionDots.find(dot => dot.isSelected) ?? selectionDots[0]
    if (!context || !highlightDate || !dot) return null
    return {
      x: context.padding.left + context.xScale(highlightDate),
      y: context.padding.top + context.yScale(dot.value),
    }
  })

  const selectionObstacles = $derived.by(() => {
    const context = lineChartContext
    if (!context) return []
    const labels = [
      ...seriesStatTooltipPlacements,
      ...aggregateLabelPlacements,
    ].flatMap(mark => {
      const bounds = statLabelBounds(mark)
      return bounds ? [bounds] : []
    })
    const markers = highlightDate
      ? selectionDots.map(dot => ({
          left: context.padding.left + context.xScale(highlightDate) - 7,
          top: context.padding.top + context.yScale(dot.value) - 7,
          width: 14,
          height: 14,
        }))
      : []
    return [...labels, ...markers]
  })
</script>

{#if visiblePointCount > 0}
  <div class="metric-time-series-chart" style:height="{height}px">
    <!-- The separate ChartKeyboardSurface provides equivalent keyboard interaction. -->
    <!-- svelte-ignore a11y_click_events_have_key_events -->
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div
      class="metric-time-series-chart__plot chart-keyboard-surface-host"
      use:measureChartAxisGutter={left => (yAxisInset = left)}
      bind:clientHeight={plotAreaHeight}
      onclick={handlePlotClick}
    >
      <MetricChartPlot
        height={plotAreaHeight > 0 ? plotAreaHeight : height}
        padding={chartPadding}
      >
        <LineChart
          bind:context={lineChartContext}
          x="date"
          y="value"
          {xScale}
          yNice
          padding={chartPadding}
          {tooltipContext}
          highlight={{ lines: true, points: false }}
          series={chartSeries}
          onPointClick={handlePointClick}
          onTooltipClick={handleTooltipClick}
          props={chartProps}
        >
          {#snippet grid()}{''}{/snippet}
          {#snippet rule({ context })}
            {#if context.yScale(0) !== Math.max(...context.yRange)}
              <Rule y={0} />
            {/if}
          {/snippet}
          {#snippet marks()}
            {#each chartSeries as series (series.key)}
              {@const emphasized =
                series.key === emphasizedSeriesKey ||
                series.key === selectedSeriesKey ||
                emphasis.has(series.key)}
              {@const aggregate = isAggregateLineKey(series.key)}
              <Spline
                seriesKey={series.key}
                curve={series.props?.curve}
                class="metric-series-line"
                data-series-key={series.key}
                data-emphasized={emphasized}
                onclick={handlePlotClick}
                strokeWidth={aggregate
                  ? emphasized
                    ? 4
                    : 2
                  : emphasized
                    ? 2.5
                    : 1.75}
                stroke-dasharray={series.props?.['stroke-dasharray']}
                style="--metric-series-line-opacity: {aggregate
                  ? emphasized
                    ? 0.95
                    : 0.35
                  : emphasized
                    ? 1
                    : emphasizedSeriesKey || selectedSeriesKey
                      ? 0.45
                      : 0.7}"
              />
            {/each}
          {/snippet}
          {#snippet tooltip({ context }: { context: any })}
            {@const xDate = tooltipXDate(context)}
            {@const rawItems = chartSeries
              .filter(s => !isAggregateLineKey(s.key))
              .slice()
              .sort((a, b) =>
                String(a.label ?? a.key).localeCompare(String(b.label ?? b.key))
              )}
            {@const aggItems = chartSeries.filter(s =>
              isAggregateLineKey(s.key)
            )}
            {@const headerLabel =
              xDate != null
                ? formatDateTime(
                    xDate.getTime(),
                    timeContext.tz,
                    'milliseconds'
                  )
                : undefined}
            <Tooltip.Root {context} class="chart-hover-card">
              {#snippet children()}
                <Tooltip.Header value={headerLabel} />
                <Tooltip.List>
                  {#each rawItems as s (s.key)}
                    {@const value =
                      xDate != null ? nearestValueAt(s.data, xDate) : undefined}
                    {#if value !== undefined}
                      <Tooltip.Item
                        {value}
                        color={s.color}
                        format={formatMetricHoverValue}
                        valueAlign="right"
                      >
                        {#snippet label()}
                          <span class="chart-series-label" title={s.label}
                            >{s.label}</span
                          >
                        {/snippet}
                      </Tooltip.Item>
                    {/if}
                  {/each}
                  {#if aggItems.length > 0 && xDate != null}
                    {@const aggregateRows = aggregateSummaryRowsAt(
                      aggItems.map(s => s.key),
                      key => {
                        const series = aggItems.find(s => s.key === key)
                        return series
                          ? nearestValueAt(series.data, xDate)
                          : undefined
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

          {#snippet belowMarks({ context }: { context: any })}
            {@const average = seriesStatMarks.find(mark => mark.kind === 'avg')}
            {#if average}
              {@const averageY = context.yScale(average.y)}
              <g
                class="series-stat-marker series-average-marker"
                class:chart-overlay--emphasized={emphasis.has('avg')}
                aria-hidden="true"
              >
                <title>{average.title}</title>
                <Line
                  x1={context.xRange[0]}
                  x2={context.xRange[1]}
                  y1={averageY}
                  y2={averageY}
                  class="series-average-line-backing"
                />
                <Line
                  x1={context.xRange[0]}
                  x2={context.xRange[1]}
                  y1={averageY}
                  y2={averageY}
                  class="series-average-line"
                />
              </g>
            {/if}
            {@const slope = seriesStatMarks.find(mark => mark.kind === 'slope')}
            {#if slope?.slopeFrom && slope.slopeTo}
              {@const guide = extendChartLine(
                {
                  x: context.xScale(slope.slopeFrom.date),
                  y: context.yScale(slope.slopeFrom.value),
                },
                {
                  x: context.xScale(slope.slopeTo.date),
                  y: context.yScale(slope.slopeTo.value),
                },
                {
                  left: Math.min(...context.xRange),
                  right: Math.max(...context.xRange),
                  top: Math.min(...context.yRange),
                  bottom: Math.max(...context.yRange),
                }
              )}
              {#if guide}
                <g
                  class="series-slope-guide"
                  class:chart-overlay--emphasized={emphasis.has('slope')}
                  aria-hidden="true"
                >
                  <title>{slope.title}</title>
                  <Line {...guide} class="series-slope-line" />
                </g>
              {/if}
            {/if}
          {/snippet}
          {#snippet aboveMarks({ context }: { context: any })}
            {@const yTop = context.yRange[1]}
            {@const yBot = context.yRange[0]}
            {#if hoveredPoint}
              <circle
                class="series-hover-point"
                cx={context.xScale(hoveredPoint.point.date)}
                cy={context.yScale(hoveredPoint.point.value)}
                r="4.5"
                style:--point-color={hoveredPoint.color}
                aria-hidden="true"
              />
            {/if}
            {#if seriesStatMarks.length > 0}
              <g
                class="series-stat-overlay"
                style:--marker-color={seriesStatMarks[0]?.color ??
                  chartSeries.find(s => s.key === selectedSeriesKey)?.color}
              >
                {#each seriesStatMarks.filter(mark => mark.kind !== 'avg') as mark (mark.kind)}
                  {@const dotPx = context.xScale(mark.dotDate)}
                  {@const dotYPx = context.yScale(mark.y)}
                  <g class="series-stat-marker">
                    <title>{mark.title}</title>
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
                    {#each selectionDots as dot (dot.key)}
                      {@const py = context.yScale(dot.value)}
                      <!-- Halo ring drawn first so the colored dot sits on
                         top. Stroke-only so the line's own color shows
                         through the center, keeping the dot readable
                         against overlapping series. -->
                      <circle
                        cx={px}
                        cy={py}
                        r="7"
                        class="selection-dot-halo"
                      />
                      <circle
                        cx={px}
                        cy={py}
                        r={dot.isSelected ? 5 : 4}
                        fill={dot.color}
                        style:--point-color={dot.color}
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
                {#each selectionDots as dot (dot.key)}
                  {@const py = context.yScale(dot.value)}
                  <circle cx={px} cy={py} r="7" class="selection-dot-halo" />
                  <circle
                    cx={px}
                    cy={py}
                    r={dot.isSelected ? 5 : 4}
                    fill={dot.color}
                    style:--point-color={dot.color}
                    class="selection-dot"
                    class:selection-dot--selected={dot.isSelected}
                  />
                {/each}
              </g>
            {/if}
            {#if keyboardCursorMark}
              {@const keyboardX = context.xScale(keyboardCursorMark.date)}
              {@const keyboardY = context.yScale(keyboardCursorMark.value)}
              <g class="chart-keyboard-cursor" aria-hidden="true">
                <Line
                  x1={keyboardX}
                  x2={keyboardX}
                  y1={yTop}
                  y2={yBot}
                  class="chart-keyboard-cursor-rule"
                />
                <circle
                  cx={keyboardX}
                  cy={keyboardY}
                  r="6"
                  fill={keyboardCursorMark.color}
                  class="chart-keyboard-cursor-point"
                />
              </g>
            {/if}
          {/snippet}
        </LineChart>
        {#if selectionLegendRows.length > 0 || selectionAggregateRows.length > 0}
          <ChartSticky anchor={selectionAnchor} obstacles={selectionObstacles}>
            <div class="metric-time-series-chart__selection-legend">
              <ChartSelectionLegend
                timestamp={selectionTimestampText}
                rows={selectionLegendRows}
                aggregateRows={selectionAggregateRows}
              />
            </div>
          </ChartSticky>
        {/if}
        {#each aggregateLabelPlacements as mark (mark.kind)}
          <div
            use:measureStatLabel={mark.kind}
            class="series-stat-tooltip series-stat-tooltip--right"
            data-aggregate-label={mark.kind}
            style:left="{mark.left}px"
            style:top="{mark.top}px"
            title={mark.title}
          >
            <div class="chart-selection-legend chart-selection-legend--stat">
              <ul class="chart-selection-legend__rows">
                <li class="chart-selection-legend__row">
                  <span
                    class="chart-selection-legend__dot"
                    style:--color={mark.color}
                    aria-hidden="true"
                  ></span>
                  <span class="chart-selection-legend__label">
                    <ChartOverlayLabel
                      label={mark.label}
                      description={mark.title}
                      active={emphasis.has(mark.kind)}
                      onclick={() => emphasis.toggle(mark.kind)}
                    />
                  </span>
                </li>
              </ul>
            </div>
          </div>
        {/each}
        {#each seriesStatTooltipPlacements as mark (mark.kind)}
          <div
            use:measureStatLabel={mark.kind}
            class="series-stat-tooltip"
            class:series-stat-tooltip--above={mark.placement === 'above'}
            class:series-stat-tooltip--below={mark.placement === 'below'}
            class:series-stat-tooltip--right={mark.placement === 'right'}
            style:left="{mark.left}px"
            style:top="{mark.top}px"
            title={mark.title}
            aria-hidden={mark.placement === 'right' ? undefined : true}
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
                    >{#if mark.placement === 'right'}<ChartOverlayLabel
                        label={mark.statLabel}
                        description={mark.title}
                        active={emphasis.has(mark.kind)}
                        onclick={() => emphasis.toggle(mark.kind)}
                      />{:else}{mark.statLabel}{/if}</span
                  >
                  {#if mark.placement !== 'right'}<span
                      class="chart-selection-legend__value"
                      >{mark.valueText}</span
                    >{/if}
                </li>
              </ul>
            </div>
          </div>
        {/each}
      </MetricChartPlot>
      <ChartKeyboardSurface
        id="metric-time-series-keyboard-surface"
        label="Metric time series chart"
        roleDescription="interactive time series chart"
        instructions={keyboardInstructions}
        readout={keyboardReadout}
        shortcuts={keyboardShortcuts}
        onKeydown={handleChartKeyboard}
        onFocusChange={handleKeyboardFocusChange}
      />
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

  .metric-time-series-chart__plot {
    @apply relative min-h-0 min-w-0 flex-1;
  }

  .metric-time-series-chart__selection-legend {
    @apply min-w-0 max-w-full;
  }

  .metric-time-series-chart :global(.metric-series-line) {
    stroke-linecap: round;
    stroke-linejoin: round;
  }

  .series-hover-point,
  .metric-time-series-chart :global(.selection-dot) {
    fill: var(--color-base-100);
    stroke: var(--point-color);
    stroke-width: 2;
    pointer-events: none;
  }

  .metric-time-series-chart :global(.selection-dot--selected) {
    fill: var(--point-color);
  }

  /* Stat cards: min/max at their points, average/slope outside the right edge. */
  .series-stat-tooltip--above {
    transform: translate(-50%, calc(-100% - 8px));
  }

  .series-stat-tooltip--below {
    transform: translate(-50%, 8px);
  }

  .series-stat-tooltip--right {
    transform: translate(8px, -50%);
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

<script module lang="ts">
  import type { MetricSummary, ScalarAggregate } from '@/types/api-types'
  import { metricSummaryKey } from '@/types/api-types'
  import { compareMetricType } from '@/components/metrics/utils/metric-type-compare'
  import {
    compareByStringField,
    compareByTimestampField,
  } from '@/utils/compare'

  export type MetricSortColumn =
    | 'name'
    | 'metricType'
    | 'serviceName'
    | 'dataPointCount'
    | 'seriesCount'
    | 'lastSeen'
  export type MetricSortDirection = 'asc' | 'desc'

  function compareMetrics(
    a: MetricSummary,
    b: MetricSummary,
    col: MetricSortColumn,
    dir: MetricSortDirection
  ): number {
    let cmp: number
    switch (col) {
      case 'name':
        cmp = compareByStringField(a, b, m => m.name)
        break
      case 'metricType':
        cmp = compareMetricType(a, b) || compareByStringField(a, b, m => m.name)
        break
      case 'serviceName':
        cmp = compareByStringField(a, b, m => m.serviceName)
        break
      case 'dataPointCount':
        cmp = a.dataPointCount - b.dataPointCount
        break
      case 'seriesCount':
        cmp = a.seriesCount - b.seriesCount
        break
      case 'lastSeen':
        cmp = compareByTimestampField(a, b, m => m.lastSeen ?? undefined)
        break
      default:
        cmp = 0
    }

    return cmp !== 0
      ? dir === 'asc'
        ? cmp
        : -cmp
      : metricSummaryKey(a).localeCompare(metricSummaryKey(b))
  }

  export { metricTypeLabel } from '@/components/metrics/utils/metric-type'
</script>

<script lang="ts">
  import { confirmDeleteAll } from '@/utils/confirm-delete'
  import {
    METRIC_BUCKET_TARGET,
    SCALAR_VIEW_BUCKETS,
    SPARKLINE_BUCKETS,
  } from '@/contexts/metric-view-context.svelte'
  import {
    DEFAULT_VISIBLE_TIMESERIES,
    persistedVisibleKeys,
  } from '@/components/metrics/utils/metric-timeseries-visible'
  import {
    DEFAULT_HISTOGRAM_QUANTILES,
    HEATMAP_BUCKET_TARGET,
  } from '@/components/metrics/utils/histogram-aggregation'
  import type { AggregateBucket, DataPoint } from '@/types/api-types'
  import { untrack } from 'svelte'
  import { createResource, createResourceMap } from '@/services/resources'
  import { createPageResources } from '@/contexts/page-resources.svelte'
  import { SvelteMap } from 'svelte/reactivity'
  import {
    telemetryAPI,
    type QueryTimeBound,
  } from '@/services/telemetry-service'
  import {
    getTimeContext,
    selectionToQueryRangeNs,
  } from '@/contexts/time-context.svelte'
  import { resolveTimezoneName, timezoneOffsetMinutes } from '@/utils/time'
  import { navigateToItem } from '@/route'
  import type { MetricViewData, MetricStats } from '@/types/api-types'
  import { createSignalListPage } from '@/contexts/signal-list-page.svelte'
  import PageLayout from '@/components/shared/PageLayout.svelte'
  import DrawerSearchPanel from '@/components/shared/Drawer/DrawerSearchPanel.svelte'
  import DrawerSortControl from '@/components/shared/Drawer/DrawerSortControl.svelte'
  import ListHeaderControls from '@/components/shared/ListHeaderControls.svelte'
  import TelemetryListFilters from '@/components/shared/TelemetryListFilters.svelte'
  import type { FieldDefinition, ParsedSearchRequest } from '@/search/model'
  import SignalDrawerFooter from '@/components/shared/Drawer/SignalDrawerFooter.svelte'
  import MetricCard from '@/components/metrics/MetricCard.svelte'
  import SignalBadges from '@/components/shared/SignalBadges.svelte'
  import MetricChartView from '@/components/metrics/Charts/MetricChartView.svelte'
  import ChartStatOverlaysToggle from '@/components/metrics/Charts/ChartStatOverlaysToggle.svelte'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { GridTableIcon, ChartHistogramIcon } from '@hugeicons/core-free-icons'
  import MetricDetailView from '@/components/metrics/Detail/MetricDetailView.svelte'
  import type { DatapointSeries } from '@/components/metrics/Detail/SeriesDatapointList.svelte'
  import SignalFooter from '@/components/shared/SignalFooter.svelte'
  import { INGESTION_ISSUES_ID } from '@/components/shared/IngestionIssues.svelte'
  import type { ImportFailure } from '@/types/import-types'
  import PaneHeader, {
    paneTabID,
  } from '@/components/shared/WorkspaceHeader.svelte'
  import ExportButton from '@/components/shared/ExportButton.svelte'
  import LoadingStatus from '@/components/shared/LoadingStatus.svelte'
  import type { AggregationView } from '@/components/metrics/utils/aggregation'
  import { aggregationViewTabs } from '@/components/metrics/utils/aggregation-view-tabs'
  import { histogramViewTabs } from '@/components/metrics/utils/histogram-view-tabs'
  import {
    createMetricViewContext,
    aggregateToSlices,
    type HistogramContributionSelection,
    getMetricViewContext,
    type HistogramTab,
  } from '@/contexts/metric-view-context.svelte'

  let {
    importFailure = null,
    onViewIssue,
  }: {
    importFailure?: ImportFailure | null
    onViewIssue?: (event: MouseEvent) => void
  } = $props()

  const METRIC_CHART_PANEL_ID = 'metric-chart-tabpanel'

  let timeContext = getTimeContext()
  const resources = createPageResources(refreshMetrics)
  const statsRead = createResource<
    Awaited<ReturnType<typeof telemetryAPI.getStats>>
  >({ scope: null })
  const metricError = (error: Error) =>
    error.message === 'The selected metric was not found.'
      ? error.message
      : 'Could not load the selected metric.'
  const summaryRead = createResource<MetricSelection>({
    formatError: metricError,
  })
  const detailRead = createResource<Parameters<typeof publishMetricDetail>[0]>({
    formatError: metricError,
  })
  type AggregateResult = Awaited<
    ReturnType<typeof telemetryAPI.getMetricAggregateView>
  >
  const aggregateRead = createResource<AggregateResult>({
    formatError: () => 'Could not load the chart aggregate.',
  })
  const aggregateSummaryRead = createResource<AggregateResult>({
    formatError: () => 'Could not load the histogram aggregate.',
  })
  const chartSeriesRead = createResource<MetricViewData | null>({
    formatError: () => 'Could not load the selected chart series.',
  })
  const contributionRead = createResource<AggregateResult>()
  const columnRead = createResource<MetricViewData | null>()

  let baselineStats = $state<MetricStats | null>(null)
  let polledStats = $state<MetricStats | null>(null)
  let actionError = $state<string | null>(null)
  let query = $state('')
  let submittedText = $state('')
  let fields = $state<FieldDefinition[]>([])
  let submittedRequest: ParsedSearchRequest | null = null
  const page = createSignalListPage<MetricSummary, MetricSortColumn>({
    signal: 'metrics',
    preserveMissingSelection: true,
    getItemID: metricSummaryKey,
    initialSort: { column: 'lastSeen', direction: 'desc' },
    compare: compareMetrics,
    fetchList: async signal => {
      // Freshness statistics run beside the list, but only a successful list
      // can establish a new baseline for the displayed results.
      statsRead.clear()
      const stats = statsRead.load(signal => telemetryAPI.getStats(signal))
      const { startTime, endTime } = selectionToQueryRangeNs(
        timeContext.selection,
        Date.now()
      )
      try {
        const results = await telemetryAPI.searchMetricSummaries(
          startTime,
          endTime,
          submittedRequest?.predicate ?? undefined,
          submittedRequest?.limit ?? undefined,
          untrack(() => ({
            field: page.sortColumn,
            direction: page.sortDirection,
          })),
          signal
        )
        if (!signal.aborted) {
          baselineStats = null
          void stats.then(() => {
            if (signal.aborted) return
            const result = statsRead.data
            if (result) {
              baselineStats = result.metrics
              polledStats = result.metrics
            }
          })
        }
        return results
      } catch (error) {
        if (!signal.aborted) statsRead.cancel()
        throw error
      }
    },
    statsPending: () => statsRead.pending,
    refreshStats: () =>
      statsRead.load(
        signal => telemetryAPI.getStats(signal),
        stats => {
          baselineStats ??= stats.metrics
          polledStats = stats.metrics
        }
      ),
  })
  const freshness = $derived.by(() => {
    if (!baselineStats || !polledStats) return { pulse: false, tip: '' }
    const parts: string[] = []
    const metricDelta = polledStats.metricCount - baselineStats.metricCount
    if (metricDelta > 0)
      parts.push(`+${metricDelta} metric${metricDelta !== 1 ? 's' : ''}`)
    const dpDelta = polledStats.dataPointCount - baselineStats.dataPointCount
    if (dpDelta > 0) parts.push(`+${dpDelta} dp${dpDelta !== 1 ? 's' : ''}`)
    return { pulse: parts.length > 0, tip: parts.join(', ') }
  })
  let services = $derived([
    ...new Set(
      page.items.flatMap(metric =>
        metric.serviceName ? [metric.serviceName] : []
      )
    ),
  ])

  // A submitted filter changes the list, not the record already open in Details.
  type MetricSelection = Pick<
    MetricSummary,
    | 'metricRef'
    | 'name'
    | 'metricType'
    | 'aggregationTemporality'
    | 'isMonotonic'
    | 'serviceName'
  >
  let retainedSummary = $state<MetricSelection>()
  const summaryLoading = $derived(summaryRead.pending)
  const summaryError = $derived(summaryRead.error ?? '')
  let summaryRetry = $state(0)
  let summaryLookupID = $state<string | null>(null)
  let selectedSummary = $derived(
    page.selectedSummary ??
      (retainedSummary?.metricRef === page.selectedID
        ? retainedSummary
        : undefined)
  )
  // Exact-identity reads start independently of the list and survive its refresh.
  $effect(() => {
    const id = page.selectedID
    if (!id || page.selectedSummary) {
      summaryLookupID = null
    } else if (page.mounted) {
      summaryLookupID = id
    } else if (untrack(() => summaryLookupID) !== id) {
      summaryLookupID = null
    }
  })
  $effect(() => {
    void summaryRetry
    summaryRead.clearError()
    const summary = page.selectedSummary
    if (summary) {
      retainedSummary = summary
      return
    }
    const id = summaryLookupID
    if (!id || untrack(() => retainedSummary?.metricRef) === id) return
    // A shared URL selects an identity independently of filtered-list membership.
    void summaryRead.load(
      async signal => {
        const metric = await telemetryAPI.getMetric(id, signal)
        if (!metric) {
          throw new Error('The selected metric was not found.')
        }
        const service = metric.resource.attributes.find(
          attribute =>
            attribute.key === 'service.name' &&
            attribute.value.kind === 'string'
        )
        const temporality =
          metric.metricType === 'Gauge'
            ? null
            : metric.aggregationTemporalityCode
        return {
          metricRef: metric.metricRef,
          name: metric.name,
          metricType: metric.metricType,
          aggregationTemporality:
            temporality === null
              ? null
              : (['Unspecified', 'Delta', 'Cumulative'][temporality] ??
                String(temporality)),
          isMonotonic: metric.metricType === 'Sum' ? metric.isMonotonic : null,
          serviceName:
            service?.value.kind === 'string' ? service.value.value : '',
        }
      },
      summary => {
        retainedSummary = summary
      }
    )
    return () => summaryRead.cancel()
  })

  let selectedMetric = $state<MetricViewData | undefined>(undefined)
  const detailLoading = $derived(detailRead.pending)
  const detailRequestKey = $derived(detailRead.requestKey)
  const detailError = $derived(detailRead.error ?? '')
  let displayedMetricSummary = $state<MetricSelection>()
  let displayedSummary = $derived(
    selectedMetric ? displayedMetricSummary : selectedSummary
  )
  // Requested series can legitimately return no reduced datapoints (for
  // example, a cumulative baseline). Keep that distinct from omitted payloads.
  let loadedDatapointSeries = $state<ReadonlySet<string>>(new Set())
  let chartDatapoints = $state.raw(new SvelteMap<string, DataPoint[]>())
  const chartSeriesLoading = $derived(chartSeriesRead.pending)
  const chartSeriesError = $derived(chartSeriesRead.error ?? '')
  const chartSeriesRequestKey = $derived(chartSeriesRead.requestKey)

  // Cross-series histogram merge for the current legend selection.
  let selectedAggregate = $state<AggregateBucket[] | null>(null)
  // The store computes the whole-window summary independently of chart columns.
  let selectedAggregateSummary = $state<AggregateBucket | null>(null)
  // Scalar aggregates for the checked and full series pools.
  let selectedScalarAggregate = $state<ScalarAggregate | null>(null)
  // Keep each independent chart answer with its exact query inputs. These are
  // references to displayed responses, not extra payloads or export data.
  type AggregateScope = {
    metric: MetricViewData
    visibleKeys: string[]
    timezoneOffsetNs: number
    timezoneName: string | undefined
    chartDatapoints: SvelteMap<string, DataPoint[]>
    loadedDatapointSeries: ReadonlySet<string>
    datapointScope: NonNullable<typeof displayedDatapointScope>
  }
  let aggregateScope = $state.raw<AggregateScope>()
  let aggregateSummaryScope = $state.raw<AggregateScope>()
  const aggregateMetric = $derived(aggregateScope?.metric)
  const aggregateSummaryMetric = $derived(aggregateSummaryScope?.metric)
  const aggregateLoading = $derived(aggregateRead.pending)
  const aggregateSummaryLoading = $derived(aggregateSummaryRead.pending)
  const aggregateError = $derived(aggregateRead.error ?? '')
  const aggregateSummaryError = $derived(aggregateSummaryRead.error ?? '')
  const aggregateRequestKey = $derived(aggregateRead.requestKey)

  // Unreduced inspected-series datapoints belong to the displayed response.
  const seriesReads = createResourceMap<string, DatapointSeries>({
    formatError: () => 'Could not load datapoints.',
  })
  const seriesDatapoints = $derived(
    new Map(
      [...seriesReads].flatMap(([key, read]) =>
        read.data ? [[key, read.data] as const] : []
      )
    )
  )
  const seriesDatapointErrors = $derived(
    new Map(
      [...seriesReads].flatMap(([key, read]) =>
        read.error ? [[key, read.error] as const] : []
      )
    )
  )
  let seriesDatapointsKey = ''
  let seriesRetry = $state(0)
  let seriesDatapointsMetricID = $derived(selectedMetric?.metricRef ?? null)
  let displayedDatapointScope = $state<{
    startTime: QueryTimeBound
    endTime: QueryTimeBound
    selection: typeof timeContext.selection
    timezone: typeof timeContext.tz
    timezoneOffsetNs: number
    timezoneName: string | undefined
    revision: number
  }>()
  const inspectedDatapointScope = $derived.by(() =>
    metricCtx.selectionSource === 'chart' &&
    metricCtx.chartMetric === aggregateMetric
      ? aggregateScope?.datapointScope
      : displayedDatapointScope
  )
  const seriesScopeKey = $derived(
    JSON.stringify([
      seriesDatapointsMetricID,
      inspectedDatapointScope?.selection,
      inspectedDatapointScope?.timezone,
      inspectedDatapointScope?.revision,
    ])
  )

  // Per-series distributions for the selected heatmap column.
  let columnDistribution = $state<MetricViewData | undefined>(undefined)
  let histogramContribution = $state<HistogramContributionSelection | null>(
    null
  )

  createMetricViewContext(
    () => selectedMetric,
    () => selectedAggregate,
    () => selectedAggregateSummary,
    () => selectedScalarAggregate,
    key => seriesDatapoints.get(key)?.datapoints,
    () => columnDistribution,
    () => histogramContribution,
    wholeWindow =>
      (wholeWindow ? aggregateSummaryMetric : aggregateMetric) ??
      selectedMetric,
    (seriesKey, source) =>
      (source === aggregateMetric
        ? aggregateScope?.chartDatapoints
        : chartDatapoints
      )?.get(seriesKey),
    () => aggregateScope?.visibleKeys
  )
  const metricCtx = getMetricViewContext()

  let hasMetricRows = $derived(page.items.length > 0)
  let displayError = $derived(page.error ?? actionError)

  let chartAggregationTabs = $derived(
    aggregationViewTabs(metricCtx.availableAggregationViews)
  )
  let lastValuesView = $state<AggregationView>('raw')
  let valuesMetricRef: string | undefined
  $effect(() => {
    const metricRef = metricCtx.metric?.metricRef
    if (metricRef !== valuesMetricRef) {
      valuesMetricRef = metricRef
      lastValuesView = 'raw'
    }
    if (metricCtx.aggregationView !== 'rate')
      lastValuesView = metricCtx.aggregationView
  })

  let showChartAggregationTabs = $derived(
    (displayedSummary?.metricType === 'Sum' ||
      displayedSummary?.metricType === 'Gauge') &&
      chartAggregationTabs.length > 1
  )

  let showChartHistogramTabs = $derived(
    displayedSummary?.metricType === 'Histogram' ||
      displayedSummary?.metricType === 'ExponentialHistogram'
  )
  let showChartOverlayControls = $derived(
    metricCtx.showChartStatOverlaysToggleVisible
  )

  let showChartTitleTabs = $derived(
    showChartAggregationTabs || showChartHistogramTabs
  )
  let activeChartTabID = $derived(
    showChartAggregationTabs
      ? metricCtx.aggregationView === 'rate'
        ? 'rate'
        : 'values'
      : metricCtx.activeHistogramTab
  )
  let chartTabPanelAttrs = $derived.by(() =>
    showChartTitleTabs
      ? {
          id: METRIC_CHART_PANEL_ID,
          role: 'tabpanel' as const,
          'aria-labelledby': paneTabID(METRIC_CHART_PANEL_ID, activeChartTabID),
          tabindex: 0,
        }
      : { id: METRIC_CHART_PANEL_ID }
  )

  // Polling may replace a summary object without changing its metric identity.
  const selectedMetricRef = $derived(selectedSummary?.metricRef)
  let detailRefresh = $state(0)
  function refreshMetrics() {
    detailRefresh++
    page.handleRefresh()
  }
  $effect(() => {
    void timeContext.selection
    void timeContext.tz
    void detailRefresh
    const requestedID = page.selectedID
    const id = selectedMetricRef
    detailRead.cancel()
    if (!id) {
      detailRead.clearError()
      if (!requestedID) selectedMetric = undefined
      return
    }
    const summary = untrack(() => selectedSummary)
    if (summary) void fetchMetricDetail(summary)
    return () => {
      detailRead.cancel()
    }
  })

  // Aggregates depend on legend selection; metric detail depends on metric identity.
  $effect(() => {
    aggregateRead.clearError()
    aggregateSummaryRead.clearError()
    const summary = displayedMetricSummary
    const metric = selectedMetric
    // Sorting keeps equivalent selection sets stable while preserving reactivity.
    const visibleKeys = [...metricCtx.visibleSeries].sort()
    const scalar =
      summary?.metricType === 'Gauge' || summary?.metricType === 'Sum'
    const needsAggregate =
      !scalar ||
      (metricCtx.aggregationView !== 'raw' && visibleKeys.length >= 2)

    // The metric response seeds the initial legend selection.
    if (
      !summary ||
      !needsAggregate ||
      !metric ||
      metric.metricRef !== summary.metricRef
    ) {
      selectedAggregate = null
      selectedAggregateSummary = null
      selectedScalarAggregate = null
      aggregateScope = undefined
      aggregateSummaryScope = undefined
      return
    }
    const bounds = effectiveMetricBounds(metric)
    if (!bounds) {
      selectedAggregate = null
      selectedAggregateSummary = null
      selectedScalarAggregate = null
      aggregateScope = undefined
      aggregateSummaryScope = undefined
      return
    }

    const datapointScope = displayedDatapointScope
    if (!datapointScope) return
    const scope: AggregateScope = {
      metric,
      visibleKeys,
      timezoneOffsetNs: displayedDatapointScope?.timezoneOffsetNs ?? 0,
      timezoneName: displayedDatapointScope?.timezoneName,
      chartDatapoints: untrack(() => chartDatapoints),
      loadedDatapointSeries: untrack(() => loadedDatapointSeries),
      datapointScope,
    }
    const requestAggregates = () => {
      void fetchAggregate(scope, bounds, false)
      if (
        summary.metricType === 'Histogram' ||
        summary.metricType === 'ExponentialHistogram'
      ) {
        void fetchAggregate(scope, bounds, true)
      }
    }
    untrack(requestAggregates)

    return () => {
      aggregateRead.cancel()
      aggregateSummaryRead.cancel()
    }
  })

  async function fetchAggregate(
    scope: AggregateScope,
    bounds: { startTime: bigint; endTime: bigint },
    wholeWindow: boolean
  ) {
    const { startTime, endTime } = bounds
    const quantiles = DEFAULT_HISTOGRAM_QUANTILES
    // Both shapes aggregate only checked series. The scoped whole pool is
    // sufficient for scalars; no second cross-series pool is needed.
    await (wholeWindow ? aggregateSummaryRead : aggregateRead).load(
      signal =>
        telemetryAPI.getMetricAggregateView(
          scope.metric.metricRef,
          startTime,
          endTime,
          wholeWindow ? 1 : HEATMAP_BUCKET_TARGET,
          scope.visibleKeys,
          quantiles,
          scope.timezoneOffsetNs,
          // Keep pooled and per-series lines on the same bucket grid.
          wholeWindow ? 0 : SCALAR_VIEW_BUCKETS,
          wholeWindow ? undefined : [],
          scope.timezoneName,
          undefined,
          signal
        ),
      result => {
        if (wholeWindow) {
          selectedAggregateSummary = result?.aggregate?.[0] ?? null
          aggregateSummaryScope = scope
        } else {
          selectedAggregate = result?.aggregate ?? null
          selectedScalarAggregate = result?.scalarAggregate ?? null
          aggregateScope = scope
        }
      }
    )
  }

  function selectMetric(key: string) {
    page.selectItem(key)
  }

  function effectiveMetricBounds(
    metric: MetricViewData
  ): { startTime: bigint; endTime: bigint } | null {
    const { startNs, endNs } = metric.window.effective
    if (startNs === null || endNs === null || endNs < startNs) return null
    return { startTime: startNs, endTime: endNs }
  }

  // Series selection loads received datapoints independently of chart visibility.
  $effect(() => {
    const scopeKey = seriesScopeKey
    const metricRef = seriesDatapointsMetricID
    const selectedSeries = metricCtx.selectedSeriesKey
    void seriesRetry
    const scope = untrack(() => inspectedDatapointScope)
    if (!metricRef || !selectedSeries || !scope) return

    const retainedChart = untrack(
      () =>
        metricCtx.selectionSource === 'chart' &&
        metricCtx.chartMetric !== selectedMetric
    )
    const now = Date.now()
    // Retained chart picks use their original query; ordinary series inspection
    // keeps anchoring relative windows when the reader requests received rows.
    const { startTime, endTime } = retainedChart
      ? scope
      : selectionToQueryRangeNs(scope.selection, now)
    const { timezone, timezoneName } = scope
    const timezoneOffsetNs = retainedChart
      ? scope.timezoneOffsetNs
      : timezoneOffsetMinutes(timezone, now) * 60 * 1_000_000_000
    // Capture every input that can change the store's answer. A structured key
    // avoids collisions with attribute-derived series and metric identifiers.
    const key = JSON.stringify([
      scopeKey,
      metricRef,
      startTime?.toString() ?? null,
      endTime?.toString() ?? null,
      timezone,
      timezoneOffsetNs,
      timezoneName ?? null,
    ])
    if (key !== seriesDatapointsKey) {
      seriesDatapointsKey = key
      seriesReads.clear()
    }

    // Cache publication must not trigger another live-window request.
    if (untrack(() => seriesDatapoints.has(selectedSeries))) return
    const read = seriesReads.for(selectedSeries)
    void read.load(signal =>
      fetchSeriesDatapoints(
        {
          metricRef,
          seriesKey: selectedSeries,
          seriesLabel: untrack(
            () =>
              metricCtx.seriesTable.labels.get(selectedSeries)?.text ??
              'default series'
          ),
          startTime,
          endTime,
          timezoneOffsetNs,
          timezoneName,
        },
        signal
      )
    )
    return () => read.cancel()
  })

  type SeriesDatapointsRequest = {
    metricRef: string
    seriesKey: string
    seriesLabel: string
    startTime: QueryTimeBound
    endTime: QueryTimeBound
    timezoneOffsetNs: number
    timezoneName: string | undefined
  }

  async function fetchSeriesDatapoints(
    request: SeriesDatapointsRequest,
    signal: AbortSignal
  ) {
    const {
      metricRef,
      seriesKey,
      seriesLabel,
      startTime,
      endTime,
      timezoneOffsetNs,
      timezoneName,
    } = request
    const result = await telemetryAPI.getMetricView(
      metricRef,
      startTime,
      endTime,
      0,
      [seriesKey],
      [],
      timezoneOffsetNs,
      undefined,
      undefined,
      undefined,
      timezoneName,
      undefined,
      undefined,
      signal
    )
    const series = result?.timeseries.find(t => t.seriesRef === seriesKey)
    // Cache terminal empty results so pending URL selections can resolve.
    return {
      metricRef,
      seriesRef: seriesKey,
      metricName: result?.name ?? '',
      seriesLabel,
      metricType: result?.metricType ?? 'Empty',
      unit: result?.unit ?? '',
      count: series?.datapointCount ?? 0,
      datapoints: series?.datapoints ?? [],
    }
  }

  /** The offset to align store-side buckets to, in nanoseconds. */
  function tzOffsetNs(now = Date.now()): number {
    return timezoneOffsetMinutes(timeContext.tz, now) * 60 * 1_000_000_000
  }

  /** Zone used to resolve bucket offsets across DST transitions. Undefined for UTC. */
  function tzName(): string | undefined {
    const name = resolveTimezoneName(timeContext.tz)
    return name === 'UTC' ? undefined : name
  }

  // Request a source contribution with the exact window, grid and checked-series
  // pool of the existing chart. Never narrow to the selected source before delta.
  $effect(() => {
    const datapointID = metricCtx.selectedDatapointID
    const seriesKey = metricCtx.selectedSeriesKey
    const wholeWindow = metricCtx.activeHistogramTab === 'histogram'
    const scope = wholeWindow ? aggregateSummaryScope : aggregateScope
    const metric = scope?.metric
    const visibleKeys = scope?.visibleKeys ?? []
    const source = metricCtx.selectionSource
    const offset = scope?.timezoneOffsetNs ?? 0
    const zone = scope?.timezoneName
    histogramContribution = null
    if (
      !metric ||
      !metricCtx.isHistogramKind ||
      source !== 'detail' ||
      !datapointID ||
      !seriesKey ||
      !visibleKeys.includes(seriesKey)
    )
      return
    const bounds = effectiveMetricBounds(metric)
    if (!bounds) return
    void contributionRead.load(
      signal =>
        telemetryAPI.getMetricAggregateView(
          metric.metricRef,
          bounds.startTime,
          bounds.endTime,
          wholeWindow ? 1 : HEATMAP_BUCKET_TARGET,
          visibleKeys,
          [],
          offset,
          0,
          undefined,
          zone,
          datapointID,
          signal
        ),
      result => {
        if (result?.contribution !== undefined) {
          histogramContribution = {
            datapointID,
            point: aggregateToSlices(result.contribution)[0] ?? null,
          }
        }
      }
    )
    return () => contributionRead.cancel()
  })

  // Fetch one merged distribution per series for the selected heatmap column.
  $effect(() => {
    // Primitive bounds avoid refetching when only the heatmap array identity changes.
    const startNs = metricCtx.heatmapColumnStartNs
    const endNs = metricCtx.heatmapColumnEndNs
    const scope = aggregateScope
    if (startNs === null || endNs === null || !scope) {
      columnDistribution = undefined
      return
    }
    // Clear stale column data while the new request is in flight.
    columnDistribution = undefined
    void columnRead.load(
      signal =>
        telemetryAPI.getMetricView(
          scope.metric.metricRef,
          // Preserve the inclusive nanosecond column boundary.
          startNs,
          endNs,
          1,
          undefined,
          DEFAULT_HISTOGRAM_QUANTILES,
          scope.timezoneOffsetNs,
          0,
          0,
          undefined,
          scope.timezoneName,
          undefined,
          undefined,
          signal
        ),
      result => {
        columnDistribution = result ?? undefined
      }
    )
    return () => columnRead.cancel()
  })

  let detailRevision = 0

  // Aggregate publication may keep the same metric and query. Depend on those
  // inputs so an arriving aggregate cannot cancel identical series work.
  const chartSeriesScope = $derived(
    metricCtx.chartMetric === aggregateMetric
      ? aggregateScope?.datapointScope
      : displayedDatapointScope
  )
  const chartSeriesPoints = $derived(
    metricCtx.chartMetric === aggregateMetric
      ? (aggregateScope?.chartDatapoints ?? chartDatapoints)
      : chartDatapoints
  )
  const chartSeriesLoaded = $derived(
    metricCtx.chartMetric === aggregateMetric
      ? (aggregateScope?.loadedDatapointSeries ?? loadedDatapointSeries)
      : loadedDatapointSeries
  )

  // Chart enrichment belongs to the displayed response, independently of a
  // different requested metric. Each checkbox change supersedes only this work.
  $effect(() => {
    const metric = metricCtx.chartMetric
    const scope = chartSeriesScope
    const points = chartSeriesPoints
    const loaded = chartSeriesLoaded
    const checked = metricCtx.visibleSeries
    const visible = [...checked].sort()
    chartSeriesRead.clearError()
    if (!metric || !scope || visible.length === 0) return

    const missing = metric.timeseries.some(
      ts =>
        checked.has(ts.seriesRef) &&
        ts.datapoints.length === 0 &&
        !loaded.has(ts.seriesRef)
    )
    if (!missing) return

    void enrichChartSeries(metric, scope, points, visible)
    return () => chartSeriesRead.cancel()
  })

  async function enrichChartSeries(
    metric: MetricViewData,
    scope: NonNullable<typeof displayedDatapointScope>,
    points: SvelteMap<string, DataPoint[]>,
    visible: string[]
  ) {
    const histogram =
      metric.metricType === 'Histogram' ||
      metric.metricType === 'ExponentialHistogram'
    await chartSeriesRead.load(
      signal =>
        telemetryAPI.getMetricView(
          metric.metricRef,
          metric.window.requested.startNs,
          metric.window.requested.endNs,
          histogram ? HEATMAP_BUCKET_TARGET : METRIC_BUCKET_TARGET,
          undefined,
          histogram ? DEFAULT_HISTOGRAM_QUANTILES : undefined,
          scope.timezoneOffsetNs,
          SCALAR_VIEW_BUCKETS,
          SPARKLINE_BUCKETS,
          undefined,
          scope.timezoneName,
          visible,
          DEFAULT_VISIBLE_TIMESERIES,
          signal
        ),
      result => {
        // Only missing chart points were requested. Do not replace metadata,
        // invalidate raw/context requests, or erase independently loaded aggregates.
        for (const series of metric.timeseries) {
          if (!visible.includes(series.seriesRef)) continue
          const received = result?.timeseries.find(
            candidate => candidate.seriesRef === series.seriesRef
          )
          if (received) points.set(series.seriesRef, received.datapoints)
        }
        if (aggregateScope?.metric === metric) {
          aggregateScope = {
            ...aggregateScope,
            loadedDatapointSeries: new Set([
              ...aggregateScope.loadedDatapointSeries,
              ...visible,
            ]),
          }
        }
        if (selectedMetric === metric)
          loadedDatapointSeries = new Set([
            ...loadedDatapointSeries,
            ...visible,
          ])
      }
    )
  }

  async function fetchMetricDetail(summary: MetricSelection) {
    const requestedDatapointSeries =
      persistedVisibleKeys(summary.metricRef) ?? undefined
    await detailRead.load(async signal => {
      const { startTime, endTime } = selectionToQueryRangeNs(
        timeContext.selection,
        Date.now()
      )
      const scope = {
        startTime,
        endTime,
        selection: timeContext.selection,
        timezone: timeContext.tz,
        timezoneOffsetNs: tzOffsetNs(),
        timezoneName: tzName(),
        revision: detailRevision + 1,
      }
      // Scalar lines and histogram heatmaps use different display resolutions.
      const isHistogramMetric =
        summary.metricType === 'Histogram' ||
        summary.metricType === 'ExponentialHistogram'
      const bucketTarget = isHistogramMetric
        ? HEATMAP_BUCKET_TARGET
        : METRIC_BUCKET_TARGET
      const result =
        (await telemetryAPI.getMetricView(
          summary.metricRef,
          startTime,
          endTime,
          bucketTarget,
          // Keep every series row; narrow only datapoints below.
          undefined,
          // Quantiles apply only to histogram bucket vectors.
          isHistogramMetric ? DEFAULT_HISTOGRAM_QUANTILES : undefined,
          // Bucket boundaries follow the reader's calendar rather than the
          // epoch. 0 is UTC, which is what the store assumes without this.
          scope.timezoneOffsetNs,
          SCALAR_VIEW_BUCKETS,
          SPARKLINE_BUCKETS,
          undefined,
          scope.timezoneName,
          // Include datapoints only for drawn series; keep all series metadata.
          requestedDatapointSeries,
          DEFAULT_VISIBLE_TIMESERIES,
          signal
        )) ?? undefined
      if (!result) {
        throw new Error('The selected metric was not found.')
      }
      return {
        metric: result,
        scope,
        summary,
        requestedDatapointSeries,
      }
    }, publishMetricDetail)
  }

  function publishMetricDetail({
    metric: result,
    scope,
    summary,
    requestedDatapointSeries,
  }: {
    metric: MetricViewData
    scope: NonNullable<typeof displayedDatapointScope>
    summary: MetricSelection
    requestedDatapointSeries: string[] | undefined
  }) {
    const changedIdentity =
      selectedMetric?.metricRef !== result.metricRef ||
      selectedMetric?.metricType !== result.metricType
    // The store's default limit uses the response's own series order. Replace
    // this set with each response so a refresh or scope change loads afresh.
    chartDatapoints = new SvelteMap()
    loadedDatapointSeries = new Set(
      requestedDatapointSeries ??
        result?.timeseries
          .slice(0, DEFAULT_VISIBLE_TIMESERIES)
          .map(ts => ts.seriesRef) ??
        []
    )
    if (changedIdentity) {
      selectedAggregate = null
      selectedAggregateSummary = null
      selectedScalarAggregate = null
      aggregateScope = undefined
      aggregateSummaryScope = undefined
    }
    selectedMetric = result
    displayedDatapointScope = scope
    detailRevision = scope.revision
    displayedMetricSummary = summary
    // Seed state before the result renders.
    if (changedIdentity) metricCtx.seedForMetric(selectedMetric)
  }

  async function handleDeleteMetric(metricRef: string) {
    actionError = null
    try {
      await telemetryAPI.deleteMetric(metricRef)
      // Clear a deleted selection before the list refetches.
      if (page.selectedID === metricRef) {
        navigateToItem('metrics', null, 'replace')
        selectedMetric = undefined
      }
      await page.runListFetch()
    } catch (err) {
      actionError =
        err instanceof Error ? err.message : 'Failed to delete metric'
    }
  }

  async function handleDeleteAllMetrics() {
    if (!confirmDeleteAll('metric')) return
    actionError = null
    try {
      await telemetryAPI.clearMetrics()
      navigateToItem('metrics', null, 'replace')
      selectedMetric = undefined
      await page.runListFetch()
    } catch (err) {
      actionError =
        err instanceof Error ? err.message : 'Failed to delete metrics'
    }
  }
</script>

<PageLayout
  mainSurface="base"
  items={page.sortedItems}
  selectedID={page.selectedID}
  drawerID="signal-drawer"
  drawerLabel="Metrics"
  onRefresh={() => void resources.refresh()}
  refreshPulse={freshness.pulse}
  refreshAsideTip={freshness.tip}
  loading={page.loading}
  loadingRequestKey={`${page.requestKey}:${page.selectedID}:${summaryRetry}:${detailRequestKey}`}
  loadingMessage={detailLoading || summaryLoading
    ? 'Loading selected metric…'
    : page.loading
      ? 'Loading metrics…'
      : undefined}
  itemKey={metricSummaryKey}
>
  {#snippet drawerSearch()}
    <DrawerSearchPanel
      segment="search"
      signal="metrics"
      sortValue={page.sortColumn}
      sortDirection={page.sortDirection}
      onSearchReady={api => (page.searchEditorApi = api)}
      onQueryChange={(text, availableFields) => {
        query = text
        fields = availableFields
      }}
      onQuerySubmit={text => {
        submittedText = text
      }}
      onSearchRequest={request => {
        submittedRequest = request
        void page.runListFetch()
      }}
    />
  {/snippet}

  {#snippet drawerControls()}
    <ListHeaderControls label="Metric list controls">
      {#snippet sort()}
        <DrawerSortControl
          options={[
            { value: 'lastSeen', label: 'Last Seen' },
            { value: 'name', label: 'Name' },
            {
              value: 'metricType',
              label: 'Type',
              description:
                'Type, then applicable temporality (Delta, Cumulative, Unspecified), then Sum monotonicity. Missing values sort last ascending.',
            },
            { value: 'serviceName', label: 'Service Name' },
            { value: 'dataPointCount', label: 'Datapoint Count' },
            { value: 'seriesCount', label: 'Timeseries Count' },
          ]}
          value={page.sortColumn}
          direction={page.sortDirection}
          onchange={page.handleSortChange}
        />
      {/snippet}
      {#snippet filters()}<TelemetryListFilters
          signal="metrics"
          {query}
          appliedQuery={submittedText}
          {fields}
          {services}
          editor={page.searchEditorApi}
        />{/snippet}
    </ListHeaderControls>
  {/snippet}

  {#snippet itemSnippet(metric, selected)}
    <MetricCard {metric} {selected} onclick={selectMetric} />
  {/snippet}

  {#snippet drawerFooter()}
    <SignalDrawerFooter label="metric" onDeleteAll={handleDeleteAllMetrics} />
  {/snippet}

  {#snippet main()}
    {#if displayedSummary}
      {#snippet metricChartHeaderBadge()}
        {#if displayedSummary}
          <SignalBadges
            signal="metric"
            metricType={displayedSummary.metricType}
            aggregationTemporality={displayedSummary.aggregationTemporality}
            isMonotonic={displayedSummary.isMonotonic}
          />
        {/if}
      {/snippet}

      {#snippet overlayControl()}
        <ChartStatOverlaysToggle ctx={metricCtx} />
      {/snippet}

      {#snippet heatmapIcon()}
        <HugeiconsIcon
          icon={GridTableIcon}
          size={16}
          strokeWidth={1.5}
          aria-hidden="true"
        />
      {/snippet}
      {#snippet histogramIcon()}
        <HugeiconsIcon
          icon={ChartHistogramIcon}
          size={16}
          strokeWidth={1.5}
          aria-hidden="true"
        />
      {/snippet}
      {@const histogramChartTabs = histogramViewTabs().map(tab => ({
        ...tab,
        icon: tab.id === 'heatmap' ? heatmapIcon : histogramIcon,
      }))}

      {#if showChartTitleTabs}
        <PaneHeader
          pill
          mode="title-tabs"
          title={displayedSummary.name}
          subtitleService={displayedSummary.serviceName || undefined}
          tabs={showChartAggregationTabs
            ? chartAggregationTabs
            : histogramChartTabs}
          activeID={activeChartTabID}
          onSelect={id => {
            if (showChartAggregationTabs) {
              metricCtx.setAggregationView(
                id === 'rate' ? 'rate' : lastValuesView
              )
            } else {
              metricCtx.setActiveHistogramTab(id as HistogramTab)
            }
          }}
          ariaLabel="Metric chart"
          tabPanelID={METRIC_CHART_PANEL_ID}
        >
          {#snippet badge()}{@render metricChartHeaderBadge()}{/snippet}
          {#snippet tabsLeading()}
            {#if showChartOverlayControls}{@render overlayControl()}{/if}
          {/snippet}
          {#snippet metaRight()}
            {#if displayedSummary}
              {#key displayedSummary.metricRef}
                <ExportButton signal="metric" id={displayedSummary.metricRef} />
              {/key}
            {/if}
          {/snippet}
        </PaneHeader>
      {:else}
        <PaneHeader
          pill
          mode="title"
          title={displayedSummary.name}
          subtitleService={displayedSummary.serviceName || undefined}
          ariaLabel="Metric chart"
        >
          {#snippet badge()}{@render metricChartHeaderBadge()}{/snippet}
          {#snippet metaRight()}
            {#if showChartOverlayControls}{@render overlayControl()}{/if}
            {#if displayedSummary}
              {#key displayedSummary.metricRef}
                <ExportButton signal="metric" id={displayedSummary.metricRef} />
              {/key}
            {/if}
          {/snippet}
        </PaneHeader>
      {/if}
    {/if}
    {#if detailError}
      <p class="metrics-page__placeholder" role="alert">{detailError}</p>
    {/if}
    {#if summaryError}
      <div
        class="metrics-page__placeholder flex items-center gap-2"
        role="alert"
      >
        <span>{summaryError}</span>
        <button
          type="button"
          class="btn btn-ghost btn-sm"
          onclick={() => summaryRetry++}>Retry</button
        >
      </div>
    {/if}
    {#if displayError || importFailure}
      <div class="metrics-page__placeholder alert alert-error" role="alert">
        {#if displayError}
          <span>Error: {displayError}</span>
        {:else if importFailure}
          <span>Import issue in {importFailure.fileName}.</span>
          <a class="link" href="/#{INGESTION_ISSUES_ID}" onclick={onViewIssue}
            >View issue</a
          >
        {/if}
      </div>
    {/if}
    {#if !selectedMetric && (displayError || importFailure || (page.loading && !hasMetricRows && !selectedSummary))}
      <div
        {...chartTabPanelAttrs}
        class="metrics-page__placeholder metrics-empty"
      ></div>
    {:else if !page.loading && !hasMetricRows && !page.selectedID}
      <div
        {...chartTabPanelAttrs}
        class="metrics-page__placeholder metrics-empty"
      >
        <p class="text-subtle">No metrics in this time range</p>
        <p class="mt-2 text-sm text-subtle">
          Send telemetry to the exporter or adjust the time range
        </p>
      </div>
    {:else}
      <div {...chartTabPanelAttrs} class="metrics-page__chart">
        <LoadingStatus
          pending={chartSeriesLoading}
          requestKey={chartSeriesRequestKey}
          label="Loading chart series…"
          class="block p-3 text-subtle"
        />
        {#if chartSeriesError}<p role="alert" class="p-3 text-papaya">
            {chartSeriesError}
          </p>{/if}
        {#if selectedMetric || !page.selectedID}<MetricChartView
            {aggregateLoading}
            {aggregateSummaryLoading}
            {aggregateError}
            {aggregateSummaryError}
            {aggregateMetric}
            {aggregateSummaryMetric}
            requestKey={aggregateRequestKey}
          />{/if}
      </div>
    {/if}
  {/snippet}

  {#snippet detail()}
    <MetricDetailView
      datapointSeries={metricCtx.selectedSeriesKey
        ? seriesDatapoints.get(metricCtx.selectedSeriesKey)
        : undefined}
      selectionCurrent={page.selectedID === selectedMetric?.metricRef}
      retryDatapoints={() => seriesRetry++}
      datapointsError={metricCtx.selectedSeriesKey
        ? seriesDatapointErrors.get(metricCtx.selectedSeriesKey)
        : undefined}
    />
  {/snippet}

  {#snippet pageFooter()}
    <SignalFooter
      index={page.selectedIndex}
      total={page.sortedItems.length}
      label="metric"
      onFirst={page.selectFirst}
      onPrev={() => page.selectByOffset(-1)}
      onNext={() => page.selectByOffset(1)}
      onLast={page.selectLast}
      onDelete={displayedSummary
        ? () => handleDeleteMetric(displayedSummary!.metricRef)
        : undefined}
    />
  {/snippet}
</PageLayout>

<style lang="postcss">
  @reference "../app.css";

  .metrics-page__chart {
    @apply flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden;
  }

  .metrics-page__placeholder {
    @apply m-[var(--layout-gap)];
  }

  .metrics-empty {
    @apply px-4 py-12 text-center;
    color: var(--color-subtle);
  }
</style>

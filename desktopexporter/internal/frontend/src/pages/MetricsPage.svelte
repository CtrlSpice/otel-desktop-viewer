<script module lang="ts">
  import type { MetricSummary } from '@/types/api-types'
  import { metricSummaryKey } from '@/types/api-types'
  import {
    compareByStringField,
    compareByTimestampField,
  } from '@/utils/compare'

  // --- Sort ---

  export type MetricSortColumn =
    | 'name'
    | 'metricType'
    | 'serviceName'
    | 'description'
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
        cmp = compareByStringField(a, b, m => m.metricType)
        break
      case 'serviceName':
        cmp = compareByStringField(a, b, m => m.serviceName)
        break
      case 'description':
        cmp = compareByStringField(a, b, m => m.description)
        break
      case 'dataPointCount':
        cmp = a.dataPointCount - b.dataPointCount
        break
      case 'seriesCount':
        cmp = a.seriesCount - b.seriesCount
        break
      case 'lastSeen':
        cmp = compareByTimestampField(a, b, m => m.lastSeen)
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

  const SORT_OPTIONS = [
    { value: 'lastSeen', label: 'Last Seen' },
    { value: 'name', label: 'Name' },
    { value: 'metricType', label: 'Type' },
    { value: 'serviceName', label: 'Service Name' },
    { value: 'description', label: 'Description' },
    { value: 'dataPointCount', label: 'Datapoint Count' },
    { value: 'seriesCount', label: 'Timeseries Count' },
  ]

  export { metricTypeBadgeClass, metricTypeLabel } from '@/components/metrics/utils/metric-type'
</script>

<script lang="ts">
  import { METRIC_BUCKET_TARGET } from '@/contexts/metric-view-context.svelte'
  import {
    HEATMAP_MAX_COLUMNS,
    localOffsetNs,
  } from '@/components/metrics/utils/histogram-aggregation'
  import type { JsonAggregateBucket } from '@/types/wire-types'
  import { untrack } from 'svelte'
  import { SvelteMap } from 'svelte/reactivity'
  import { telemetryAPI } from '@/services/telemetry-service'
  import {
    getTimeContext,
    isDefaultUnboundedWindow,
    selectionToQueryRangeMs,
  } from '@/contexts/time-context.svelte'
  import { navigateToItem } from '@/route'
  import type { DataPoint, MetricData, MetricStats } from '@/types/api-types'
  import { createSignalListPage } from '@/contexts/signal-list-page.svelte'
  import PageLayout from '@/components/shared/PageLayout.svelte'
  import DrawerSearchPanel from '@/components/shared/Drawer/DrawerSearchPanel.svelte'
  import SignalDrawerFooter from '@/components/shared/Drawer/SignalDrawerFooter.svelte'
  import MetricCard from '@/components/metrics/MetricCard.svelte'
  import SignalBadges from '@/components/shared/SignalBadges.svelte'
  import MetricChartView from '@/components/metrics/Charts/MetricChartView.svelte'
  import MetricDetailView from '@/components/metrics/Detail/MetricDetailView.svelte'
  import SignalFooter from '@/components/shared/SignalFooter.svelte'
  import PaneHeader from '@/components/shared/PaneHeader.svelte'
  import type { AggregationView } from '@/components/metrics/utils/aggregation'
  import { aggregationViewTabs } from '@/components/metrics/utils/aggregation-view-tabs'
  import { histogramViewTabs } from '@/components/metrics/utils/histogram-view-tabs'
  import {
    createMetricViewContext,
    getMetricViewContext,
    type HistogramTab,
  } from '@/contexts/metric-view-context.svelte'

  let timeContext = getTimeContext()

  let baselineStats = $state<MetricStats | null>(null)
  let polledStats = $state<MetricStats | null>(null)
  let actionError = $state<string | null>(null)

  const page = createSignalListPage<MetricSummary>({
    signal: 'metrics',
    getItemId: metricSummaryKey,
    initialSort: { column: 'lastSeen', direction: 'desc' },
    compare: (a, b, col, dir) =>
      compareMetrics(a, b, col as MetricSortColumn, dir as MetricSortDirection),
    fetchList: async () => {
      const { start: startTime, end: endTime } = selectionToQueryRangeMs(
        timeContext.selection,
        Date.now()
      )
      const results = await telemetryAPI.searchMetricSummaries(startTime, endTime)
      const s = await telemetryAPI.getStats()
      baselineStats = s.metrics
      polledStats = s.metrics
      return results
    },
    pollStats: async () => {
      const s = await telemetryAPI.getStats()
      polledStats = s.metrics
    },
    refreshFromStats: () => {
      if (!baselineStats || !polledStats) {
        return { pulse: false, tip: '' }
      }
      const parts: string[] = []
      const metricDelta = polledStats.metricCount - baselineStats.metricCount
      if (metricDelta > 0) {
        parts.push(`+${metricDelta} metric${metricDelta !== 1 ? 's' : ''}`)
      }
      const dpDelta = polledStats.dataPointCount - baselineStats.dataPointCount
      if (dpDelta > 0) {
        parts.push(`+${dpDelta} dp${dpDelta !== 1 ? 's' : ''}`)
      }
      return { pulse: parts.length > 0, tip: parts.join(', ') }
    },
  })

  let selectedMetric = $state<MetricData | undefined>(undefined)
  let detailLoading = $state(false)

  // The store's cross-series merge for the current legend selection. Null
  // until the first fetch resolves, and whenever the metric changes.
  let selectedAggregate = $state<JsonAggregateBucket[] | null>(null)
  // The same merge over one bucket spanning the window. A window summary is
  // not the last column of a chart, and it is not the columns added together
  // either -- it is the merge asked a different question, so the store answers
  // it rather than the client approximating from what it already has.
  let selectedAggregateSummary = $state<JsonAggregateBucket | null>(null)

  // Unreduced datapoints per series, fetched when a series is expanded.
  //
  // Keyed by series id, cleared whenever the metric or the window changes so a
  // stale series can never be shown under a new question. A SvelteMap because
  // the list reads it during render.
  let seriesDatapoints = new SvelteMap<string, DataPoint[]>()
  let seriesDatapointsKey = $state('')

  createMetricViewContext(
    () => selectedMetric,
    () => selectedAggregate,
    () => selectedAggregateSummary,
    key => seriesDatapoints.get(key)
  )
  const metricCtx = getMetricViewContext()

  let hasMetricRows = $derived(page.items.length > 0)
  let displayError = $derived(page.error ?? actionError)

  let chartAggregationTabs = $derived(
    aggregationViewTabs(metricCtx.availableAggregationViews)
  )

  let showChartAggregationTabs = $derived(
    (page.selectedSummary?.metricType === 'Sum' ||
      page.selectedSummary?.metricType === 'Gauge') &&
      chartAggregationTabs.length > 1
  )

  let showChartHistogramTabs = $derived(
    page.selectedSummary?.metricType === 'Histogram' ||
      page.selectedSummary?.metricType === 'ExponentialHistogram'
  )

  let showChartTitleTabs = $derived(
    showChartAggregationTabs || showChartHistogramTabs
  )

  // Re-fetch metric detail ONLY when the selected metric's identity changes --
  // not when the summary object reference churns. Polling rebuilds the list
  // every few seconds with fresh object references; depending on the summary
  // object directly would re-fetch on every poll and clobber per-metric view
  // state (AggregationView, legend selections, etc.).
  $effect(() => {
    const id = page.selectedSummary
      ? metricSummaryKey(page.selectedSummary)
      : null
    if (!id) {
      selectedMetric = undefined
      return
    }
    const summary = untrack(() => page.selectedSummary)
    if (summary) fetchMetricDetail(summary)
  })

  // The aggregate is fetched separately from the metric, because the two have
  // different lifetimes. Per-series quantiles are additive -- fetched once with
  // the metric, and any subset's lines are already in hand -- while the merge
  // is specific to the selection and has to be recomputed when the legend
  // changes.
  //
  // Folding this into the detail effect would break that effect's contract: it
  // depends on the metric identity alone and untracks the summary, because
  // polling churns object references and re-fetching would clobber legend
  // state. It would also fetch twice per metric, since the legend selection is
  // seeded from the response it would then depend on.
  let aggregateTimer: ReturnType<typeof setTimeout> | undefined
  let aggregateToken = 0

  $effect(() => {
    const summary = page.selectedSummary
    // Read reactively so a toggle re-runs this. Sorted so a set rebuilt with
    // the same members does not look like a change.
    const keys = [...metricCtx.histogramVisible].sort()

    // Wait for the metric itself. The legend selection is seeded from that
    // response, so fetching before it arrives asks for the empty set -- which
    // now correctly means "no series" and returns nothing.
    if (!summary || !metricCtx.isHistogramKind || !selectedMetric) {
      selectedAggregate = null
      selectedAggregateSummary = null
      return
    }

    // Coalesce rapid toggles into one request. Ticking through five series
    // should ask the store once, not five times.
    clearTimeout(aggregateTimer)
    const token = ++aggregateToken
    aggregateTimer = setTimeout(() => {
      void fetchAggregate(summary, keys, token)
    }, 120)

    return () => clearTimeout(aggregateTimer)
  })

  async function fetchAggregate(
    summary: MetricSummary,
    keys: string[],
    token: number
  ) {
    try {
      const { start: startTime, end: endTime } = selectionToQueryRangeMs(
        timeContext.selection,
        Date.now()
      )
      // No quantiles: nothing here reads the ones the store computes.
      //
      // Every quantile this page draws is derived client-side from the bucket
      // vectors -- the Quantiles tab through sliceQuantileValue, a heatmap
      // column through heatmapColumnSelectionAt, the distribution marks
      // through quantilePointSelectionAt -- all of them calling
      // histogramQuantilesForDatapoint on data already in hand. The server's
      // quantiles field was arriving and being discarded.
      //
      // It was not free. Measured on interval_distribution over 21 series at
      // 400 buckets: 31,623 ms asking for three quantiles against 285 ms
      // asking for none, for 14% more payload. Asking for what is read costs
      // a hundredth of asking for what is not.
      const quantiles: number[] = []
      // Same answer as the detail fetch gives, so the aggregate is bucketed
      // over the window the series beneath it were bucketed over.
      const fit = isDefaultUnboundedWindow(timeContext.selection)
      // Both shapes of the same question, issued together so they cannot
      // disagree about the window or the selection.
      const [buckets, whole] = await Promise.all([
        telemetryAPI.getMetricAggregate(
          summary.id,
          startTime,
          endTime,
          HEATMAP_MAX_COLUMNS,
          keys,
          quantiles,
          tzOffsetNs(),
          fit
        ),
        telemetryAPI.getMetricAggregate(
          summary.id,
          startTime,
          endTime,
          1,
          keys,
          quantiles,
          tzOffsetNs(),
          fit
        ),
      ])
      // A slower earlier request must not overwrite a newer answer.
      if (token === aggregateToken) {
        selectedAggregate = buckets
        selectedAggregateSummary = whole?.[0] ?? null
      }
    } catch (err) {
      console.error('Failed to fetch metric aggregate:', err)
      if (token === aggregateToken) {
        selectedAggregate = null
        selectedAggregateSummary = null
      }
    }
  }

  function selectMetric(key: string) {
    page.selectItem(key)
  }

  // Expanding a series fetches that series as it was sent: no reduction, no
  // quantiles, one series.
  //
  // The chart may show a reduction -- that is what a chart is for. The list may
  // not: it is the view that answers "what did my service actually send", and
  // for a reduced histogram `metric.timeseries[].datapoints` are the store's
  // merged buckets rather than datapoints. On the reference capture the window
  // held 17,076 and the response carried 3,094.
  //
  // Fetched on expansion rather than with the metric because it is the only
  // place that wants every row, and narrow enough to be cheap: the store
  // filters by series before reducing, so one series of that same metric is
  // 1,001 datapoints in 132 ms. Asking for all of them up front would put the
  // whole unreduced stream on the wire to render twenty rows.
  $effect(() => {
    const summary = page.selectedSummary
    const expanded = [...metricCtx.expandedTimeseries]
    const { start, end } = selectionToQueryRangeMs(
      timeContext.selection,
      Date.now()
    )
    // One key per (metric, window). When it changes every cached series is
    // answering a question nobody asked any more.
    const key = summary ? `${summary.id}:${start}:${end}:${timeContext.tz}` : ''
    if (key !== seriesDatapointsKey) {
      seriesDatapointsKey = key
      seriesDatapoints.clear()
    }
    if (!summary || expanded.length === 0) return

    for (const seriesKey of expanded) {
      if (seriesDatapoints.has(seriesKey)) continue
      void fetchSeriesDatapoints(summary.id, seriesKey, start, end, key)
    }
  })

  const seriesInFlight = new Set<string>()

  async function fetchSeriesDatapoints(
    streamId: string,
    seriesKey: string,
    startTime: number,
    endTime: number,
    cacheKey: string
  ) {
    if (seriesInFlight.has(seriesKey)) return
    seriesInFlight.add(seriesKey)
    try {
      const result = await telemetryAPI.getMetric(
        streamId,
        startTime,
        endTime,
        // No reduction. This is the request the whole feature is about.
        0,
        [seriesKey],
        // None: the list shows what arrived, and quantiles are a derived
        // statistic the client computes anyway.
        [],
        tzOffsetNs(),
        isDefaultUnboundedWindow(timeContext.selection)
      )
      // The window may have moved while this was in flight; a late answer to a
      // superseded question must not land in the new cache.
      if (cacheKey !== seriesDatapointsKey) return
      const series = result?.timeseries.find(t => t.attributesKey === seriesKey)
      if (series) seriesDatapoints.set(seriesKey, series.datapoints)
    } catch (err) {
      console.error('Failed to fetch series datapoints:', err)
    } finally {
      seriesInFlight.delete(seriesKey)
    }
  }

  /** The offset to align store-side buckets to, in nanoseconds. */
  function tzOffsetNs(): number {
    if (timeContext.tz === 'UTC') return 0
    return Number(localOffsetNs(BigInt(Date.now()) * 1_000_000n))
  }

  async function fetchMetricDetail(summary: MetricSummary) {
    try {
      detailLoading = true
      const { start: startTime, end: endTime } = selectionToQueryRangeMs(
        timeContext.selection,
        Date.now()
      )
      // Ask the store to reduce to roughly one bucket per chart point it will
      // draw. It keeps up to four per bucket -- first, last, min, max -- so the
      // line is the same as it would be with every datapoint, and it ignores
      // the request for histograms, which need merging rather than sampling.
      //
      // This is what stops a dense stream shipping tens of megabytes to draw a
      // few thousand pixels: measured at 46.4 MB and 640 ms down to 0.36 MB and
      // 60 ms on a 242,324-datapoint stream.
      // A histogram is drawn as a heatmap, which cannot show more columns than
      // it has room for -- so ask for the heatmap's ceiling rather than the
      // line chart's target. The difference is not free: histogram datapoints
      // carry a bucket vector and a quantile set each, so the line-chart target
      // fetches five times the payload and discards four fifths of it at the
      // merge. It only became visible once the window stopped collapsing to a
      // single bucket, which had been hiding the cost.
      const bucketTarget =
        summary.metricType === 'Histogram' ||
        summary.metricType === 'ExponentialHistogram'
          ? HEATMAP_MAX_COLUMNS
          : METRIC_BUCKET_TARGET
      selectedMetric =
        (await telemetryAPI.getMetric(
          summary.id,
          startTime,
          endTime,
          bucketTarget,
          // Every series for now. Narrowing to the legend selection needs the
          // fetch to re-run on toggle, which is a separate change.
          undefined,
          // None, for the reason given in fetchAggregate: the client derives
          // every quantile it draws from the bucket vectors it already has.
          [],
          // Bucket boundaries follow the reader's calendar rather than the
          // epoch. 0 is UTC, which is what the store assumes without this.
          tzOffsetNs(),
          // "All" is the absence of a choice, so the store divides the data's
          // own extent instead of the window. Without this the reduction
          // divided decades: a two-hour session came back as a single bucket
          // per series, and no amount of client-side axis fitting could put
          // back the resolution that was never sent.
          isDefaultUnboundedWindow(timeContext.selection)
        )) ?? undefined
    } catch (err) {
      console.error('Failed to fetch metric detail:', err)
      selectedMetric = undefined
    } finally {
      detailLoading = false
    }
  }

  async function handleDeleteMetric(streamID: string) {
    actionError = null
    try {
      await telemetryAPI.deleteMetricStream(streamID)
      // Clear the detail pane before refetching: the $effect above keys off
      // page.selectedSummary, and the deleted stream is gone from the next
      // list fetch, so leaving it set would render a stale chart.
      if (page.selectedId === streamID) {
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

<div class="metrics-page">
  <PageLayout
    items={page.sortedItems}
    selectedId={page.selectedId}
    drawerId="signal-drawer"
    drawerLabel="Metrics"
    onRefresh={page.handleRefresh}
    refreshPulse={page.refreshPulse}
    refreshAsideTip={page.refreshAsideTip}
    loading={page.loading}
    itemKey={metricSummaryKey}
    resizableStorageKey="metric-detail-panels"
    minDetailPx={352}
  >
    {#snippet drawerChromeToolbar()}
      <DrawerSearchPanel
        segment="toolbar"
        signal="metrics"
        sortOptions={SORT_OPTIONS}
        sortValue={page.sortColumn}
        sortDirection={page.sortDirection}
        onSortChange={page.handleSortChange}
      />
    {/snippet}

    {#snippet drawerSearch()}
      <DrawerSearchPanel
        segment="search"
        signal="metrics"
        sortOptions={SORT_OPTIONS}
        sortValue={page.sortColumn}
        sortDirection={page.sortDirection}
        onSortChange={page.handleSortChange}
        onSearchResults={page.handleSearchResults}
        onSearchReady={api => (page.searchEditorApi = api)}
      />
    {/snippet}

    {#snippet itemSnippet(metric, selected)}
      <MetricCard {metric} {selected} onclick={selectMetric} />
    {/snippet}

    {#snippet drawerFooter()}
      <SignalDrawerFooter
        count={page.sortedItems.length}
        label="metric"
        onDeleteAll={handleDeleteAllMetrics}
      />
    {/snippet}

    {#snippet main()}
      <!-- Page-level error / empty branches replace the chart pane
           entirely; the chart lives here on the happy path and the
           detail pane (Fields/Series) renders alongside. The
           SignalFooter is now page-level chrome (see pageFooter
           snippet below): always present, spans main + detail
           regardless of content state, and DetailNav self-disables
           when there is nothing to navigate. -->
        {#if page.selectedSummary}
        {@const selectedSummary = page.selectedSummary}
        {#snippet metricChartHeaderBadge()}
          <SignalBadges
            signal="metric"
            metricType={selectedSummary.metricType}
            aggregationTemporality={selectedSummary.aggregationTemporality}
            isMonotonic={selectedSummary.isMonotonic}
          />
        {/snippet}

        {@const histogramChartTabs = histogramViewTabs()}

        {#if showChartTitleTabs}
          <PaneHeader
            mode="title-tabs"
            title={selectedSummary.name}
            subtitle={selectedSummary.serviceName?.trim() || undefined}
            tabs={showChartAggregationTabs
              ? chartAggregationTabs
              : histogramChartTabs}
            activeId={showChartAggregationTabs
              ? metricCtx.aggregationView
              : metricCtx.activeHistogramTab}
            onSelect={id => {
              if (showChartAggregationTabs) {
                metricCtx.setAggregationView(id as AggregationView)
              } else {
                metricCtx.setActiveHistogramTab(id as HistogramTab)
              }
            }}
            ariaLabel="Metric chart"
          >
            {#snippet badge()}{@render metricChartHeaderBadge()}{/snippet}
          </PaneHeader>
        {:else}
          <PaneHeader
            mode="title"
            title={selectedSummary.name}
            subtitle={selectedSummary.serviceName?.trim() || undefined}
            ariaLabel="Metric chart"
          >
            {#snippet badge()}{@render metricChartHeaderBadge()}{/snippet}
          </PaneHeader>
        {/if}
      {/if}
        {#if displayError}
          <div class="metrics-page__placeholder alert alert-error">
            <span>Error: {displayError}</span>
          </div>
        {:else if page.loading && !hasMetricRows}
          <div class="metrics-page__placeholder metrics-empty">
            Loading metrics…
          </div>
        {:else if !page.loading && !hasMetricRows}
          <div class="metrics-page__placeholder metrics-empty">
            <p class="text-rp-subtle">No metrics in this time range</p>
            <p class="mt-2 text-sm text-rp-muted">
              Send telemetry to the exporter or adjust the time range
            </p>
          </div>
        {:else}
          <div class="metrics-page__chart">
            <MetricChartView />
          </div>
      {/if}
    {/snippet}

    {#snippet detail()}
      <MetricDetailView />
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
        onDelete={page.selectedSummary
          ? () => handleDeleteMetric(page.selectedSummary!.id)
          : undefined}
      />
    {/snippet}
  </PageLayout>
</div>

<style lang="postcss">
  @reference "../app.css";

  .metrics-page {
    @apply flex min-h-0 min-w-0 w-full flex-1 flex-col;
  }

  .metrics-page__chart {
    @apply flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden;
  }

  /*
   * Page-level placeholder branches (error / loading / empty list).
   * Take the full main pane so the surrounding chrome already
   * provides the card framing -- no double-card.
   */
  .metrics-page__placeholder {
    @apply m-[var(--layout-gap)];
  }

  .metrics-empty {
    @apply px-4 py-12 text-center;
    color: var(--color-subtle);
  }
</style>

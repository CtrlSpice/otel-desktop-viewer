<script lang="ts">
  import { SvelteSet } from 'svelte/reactivity'
  import type {
    MetricData,
    DataPoint,
    HistogramDataPoint,
    ExponentialHistogramDataPoint,
    BucketSeriesPoint,
    HistogramBucketPoint,
    ExpHistogramBucketPoint,
  } from '@/types/api-types'
  import { formatTimestamp } from '@/utils/time'
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import { selectionToQueryRangeMs } from '@/contexts/time-context.svelte'
  import {
    telemetryAPI,
    JsonRpcError,
    ErrCodeUnspecifiedTemporality,
    ErrCodeHistogramBoundsMismatch,
  } from '@/services/telemetry-service'
  import MetricTimeSeriesChart from '@/components/MetricCharts/MetricTimeSeriesChart.svelte'
  import HistogramChart from '@/components/MetricCharts/HistogramChart.svelte'
  import HistogramHeatmap from '@/components/MetricCharts/HistogramHeatmap.svelte'
  import UnspecifiedTemporalityCallout from '@/components/MetricDetails/UnspecifiedTemporalityCallout.svelte'
  import MetricField from './MetricField.svelte'
  import ResizablePanels from '@/components/ResizablePanels.svelte'
  import DetailNav from '@/components/DetailNav.svelte'
  import { TrashIcon } from '@/icons'

  type Props = {
    metric: MetricData | undefined
    // Per-metric delete (currently unused by MetricsPage; kept so the
    // panel footer's trash button stays wired to a real callback once
    // the page opts in).
    onDelete?: (id: string) => void
    // Position of this metric in the parent's sorted list, used by the
    // DetailNav prev/next/first/last controls in the footer. Optional
    // so callers that don't need nav (tests, embedding) can skip them;
    // when total is 0 / index is -1, DetailNav renders all buttons
    // disabled, which is the right empty-state behaviour.
    index?: number
    total?: number
    onFirst?: () => void
    onPrev?: () => void
    onNext?: () => void
    onLast?: () => void
  }

  let {
    metric,
    onDelete,
    index = -1,
    total = 0,
    onFirst,
    onPrev,
    onNext,
    onLast,
  }: Props = $props()

  let timeContext = getTimeContext()

  let metricType = $derived(metric?.datapoints[0]?.metricType ?? 'Empty')

  // aggregationTemporality lives on each datapoint (Sum / Histogram /
  // ExpHist). Per OTLP they all agree within a single MetricData; we
  // sample the first non-empty one so an empty leading dp doesn't blank
  // out the field. Returns '' for Gauge or for metrics that don't carry
  // it -- caller decides whether to render the row.
  let temporality = $derived.by(() => {
    if (!metric) return ''
    for (const dp of metric.datapoints) {
      const t = (dp as { aggregationTemporality?: string }).aggregationTemporality
      if (t) return t
    }
    return ''
  })

  // Sum-only. Same scan strategy as temporality. Returned as the literal
  // string OTLP gives us so the field row reads naturally; null means
  // "not a Sum, don't render the row".
  let isMonotonic = $derived.by((): boolean | null => {
    if (!metric || metricType !== 'Sum') return null
    for (const dp of metric.datapoints) {
      if (dp.metricType === 'Sum') return dp.isMonotonic
    }
    return null
  })

  type MetadataAttr = {
    key: string
    value: string
    type: string
    scope: 'resource' | 'scope'
  }

  let resourceAttrs = $derived.by((): MetadataAttr[] => {
    if (!metric) return []
    return metric.resource.attributes.map(a => ({
      key: a.key,
      value: a.value,
      type: a.type,
      scope: 'resource' as const,
    }))
  })

  let scopeAttrs = $derived.by((): MetadataAttr[] => {
    if (!metric) return []
    const out: MetadataAttr[] = []
    if (metric.scope.name) {
      out.push({ key: 'name', value: metric.scope.name, type: 'string', scope: 'scope' })
    }
    if (metric.scope.version) {
      out.push({ key: 'version', value: metric.scope.version, type: 'string', scope: 'scope' })
    }
    for (const a of metric.scope.attributes) {
      out.push({ key: a.key, value: a.value, type: a.type, scope: 'scope' })
    }
    return out
  })

  // Per OTLP, temporality lives at the metric definition level so every
  // datapoint in a single MetricData should agree. We scan the whole
  // list anyway -- if any datapoint reports 'Unspecified' the FunError
  // fires. Cheap (linear in datapoint count) and means a malformed/mixed
  // payload can't sneak past by hiding the bad temporality in [1..n].
  let isUnspecifiedTemporality = $derived.by(() => {
    if (
      metricType !== 'Histogram' &&
      metricType !== 'ExponentialHistogram' &&
      metricType !== 'Sum'
    ) {
      return false
    }
    if (!metric) return false
    for (const dp of metric.datapoints) {
      const t = (dp as { aggregationTemporality?: string }).aggregationTemporality
      if (t === 'Unspecified') return true
    }
    return false
  })

  let isHistogramKind = $derived(
    metricType === 'Histogram' || metricType === 'ExponentialHistogram'
  )

  let queryRange = $derived(selectionToQueryRangeMs(timeContext.selection, Date.now()))

  // -- Selected datapoint state ----------------------------------------
  // Drives the snapshot chart (histograms), the time-series highlight
  // (Gauge/Sum), and the persistent column highlight on the heatmap.
  // Reset whenever the metric changes so a stale id from a previous
  // selection doesn't carry over.
  let selectedDatapointId = $state<string | null>(null)

  // Active tab for histogram/exphist. Three tabs share the chart slot:
  // Heatmap (default), Aggregated (whole-window summary bar chart),
  // Snapshot (per-datapoint bar chart). For Gauge/Sum this state is
  // unused -- the chart slot just shows the time-series chart with a
  // "Time series" section label instead of a tab strip.
  let activeTab = $state<'heatmap' | 'aggregated' | 'snapshot'>('heatmap')

  // Reset selection + tab whenever the metric changes. Reading metric.id
  // inside the effect ties it to the right reactive dependency.
  $effect(() => {
    void metric?.id
    selectedDatapointId = null
    activeTab = 'heatmap'
  })

  // Resolved selected datapoint object (or undefined if no selection /
  // selection no longer in the list).
  let selectedDatapoint = $derived.by(() => {
    if (!metric || !selectedDatapointId) return undefined
    return metric.datapoints.find(dp => dp.id === selectedDatapointId)
  })

  let latestHistogramDp = $derived.by(() => {
    if (!metric || !isHistogramKind) return undefined
    const sorted = [...metric.datapoints]
      .filter(dp => dp.metricType === 'Histogram' || dp.metricType === 'ExponentialHistogram')
      .sort((a, b) => (a.timestamp > b.timestamp ? -1 : 1))
    return sorted[0] as HistogramDataPoint | ExponentialHistogramDataPoint | undefined
  })

  // Histogram snapshot honours the selected datapoint; falls back to
  // the latest if none selected, preserving the prior default behaviour.
  let activeHistogramDp = $derived.by(() => {
    if (
      selectedDatapoint &&
      (selectedDatapoint.metricType === 'Histogram' ||
        selectedDatapoint.metricType === 'ExponentialHistogram')
    ) {
      return selectedDatapoint as HistogramDataPoint | ExponentialHistogramDataPoint
    }
    return latestHistogramDp
  })

  // For the time-series highlight (Gauge/Sum). Pull only when a
  // Gauge/Sum is selected; otherwise leave null so the rule doesn't
  // render.
  let highlightedTimestamp = $derived.by(() => {
    if (
      selectedDatapoint &&
      (selectedDatapoint.metricType === 'Gauge' ||
        selectedDatapoint.metricType === 'Sum')
    ) {
      return selectedDatapoint.timestamp
    }
    return null
  })

  // -- Bucket series fetch (lifted from HistogramHeatmap) --------------
  // Both the Heatmap tab AND the Aggregated tab consume bucket data, so
  // we fetch once at this level and pass points down. The heatmap gets
  // the full per-time-bucket series; the aggregated tab gets a separate
  // single-bucket call (maxPoints=1) so the backend's alignment pipeline
  // produces the merged-across-time-and-streams vector for free.
  //
  // Errors are categorized so all three tabs can show a consistent
  // message (and the Unspecified-temporality FunError still fires from
  // the dp-level check above; this typed-error path is defence in depth
  // against backend disagreements).
  type BucketSeriesError =
    | { kind: 'unspecified'; message: string }
    | { kind: 'boundsMismatch'; message: string }
    | { kind: 'other'; message: string }

  let bucketSeries = $state<BucketSeriesPoint[] | null>(null)
  let bucketSeriesLoading = $state(false)
  let bucketSeriesError = $state<BucketSeriesError | null>(null)

  let aggregatedPoint = $state<BucketSeriesPoint | null>(null)
  let aggregatedLoading = $state(false)
  let aggregatedError = $state<BucketSeriesError | null>(null)

  // Re-fetch when the metric or window changes. Both calls share the
  // same window + mode but differ in maxPoints, so we issue them in
  // parallel rather than chaining. cancelled flag stops late responses
  // from a prior metric from clobbering the current state.
  $effect(() => {
    const id = metric?.id
    const t = metricType
    const start = queryRange.start
    const end = queryRange.end

    if (!id || (t !== 'Histogram' && t !== 'ExponentialHistogram')) {
      bucketSeries = null
      bucketSeriesError = null
      bucketSeriesLoading = false
      aggregatedPoint = null
      aggregatedError = null
      aggregatedLoading = false
      return
    }

    let cancelled = false
    bucketSeries = null
    bucketSeriesError = null
    bucketSeriesLoading = true
    aggregatedPoint = null
    aggregatedError = null
    aggregatedLoading = true

    telemetryAPI
      .getMetricBucketSeries(id, 'aggregated', start, end, 100)
      .then(result => {
        if (!cancelled) {
          bucketSeries = result
          bucketSeriesLoading = false
        }
      })
      .catch(err => {
        if (cancelled) return
        bucketSeriesLoading = false
        bucketSeriesError = categorizeBucketSeriesError(err)
      })

    // Single-bucket call: backend collapses time_merged across the full
    // window, then aggregates across streams, producing exactly one
    // BucketSeriesPoint with the merged vector. Reusing the existing
    // path means we don't reimplement ExpHist alignment in TS.
    telemetryAPI
      .getMetricBucketSeries(id, 'aggregated', start, end, 1)
      .then(result => {
        if (!cancelled) {
          aggregatedPoint = result.length > 0 ? result[0] : null
          aggregatedLoading = false
        }
      })
      .catch(err => {
        if (cancelled) return
        aggregatedLoading = false
        aggregatedError = categorizeBucketSeriesError(err)
      })

    return () => {
      cancelled = true
    }
  })

  function categorizeBucketSeriesError(err: unknown): BucketSeriesError {
    if (err instanceof JsonRpcError) {
      if (err.code === ErrCodeUnspecifiedTemporality) {
        return { kind: 'unspecified', message: err.message }
      }
      if (err.code === ErrCodeHistogramBoundsMismatch) {
        return { kind: 'boundsMismatch', message: err.message }
      }
    }
    return {
      kind: 'other',
      message: err instanceof Error ? err.message : String(err),
    }
  }

  // -- Aggregated synthetic datapoint ----------------------------------
  // Wraps the single aggregated BucketSeriesPoint into the
  // HistogramDataPoint / ExponentialHistogramDataPoint shape that
  // HistogramChart consumes. No new math here -- the backend did the
  // merging; we're just translating between row schemas.
  //
  // The synthetic id is the metric id with an :aggregated suffix. It
  // never hits the backend (HistogramChart in 'aggregated' mode skips
  // the per-dp quantile fetch entirely), so it just needs to be stable
  // enough for any downstream id-keyed memoization.
  let aggregatedDatapoint = $derived.by(():
    | HistogramDataPoint
    | ExponentialHistogramDataPoint
    | undefined => {
    if (!aggregatedPoint || !metric) return undefined
    const id = `${metric.id}:aggregated`
    if (aggregatedPoint.kind === 'histogram') {
      const p = aggregatedPoint as HistogramBucketPoint
      return {
        id,
        metricType: 'Histogram',
        timestamp: p.timestamp,
        startTime: p.timestamp,
        attributes: [],
        flags: 0,
        exemplars: [],
        count: p.totals.count,
        sum: p.totals.sum,
        min: p.totals.min ?? 0,
        max: p.totals.max ?? 0,
        explicitBounds: p.bounds,
        bucketCounts: p.counts,
        aggregationTemporality: temporality || 'Delta',
      }
    }
    const p = aggregatedPoint as ExpHistogramBucketPoint
    return {
      id,
      metricType: 'ExponentialHistogram',
      timestamp: p.timestamp,
      startTime: p.timestamp,
      attributes: [],
      flags: 0,
      exemplars: [],
      count: p.totals.count,
      sum: p.totals.sum,
      min: p.totals.min ?? 0,
      max: p.totals.max ?? 0,
      scale: p.scale,
      zeroCount: p.zeroCount,
      zeroThreshold: p.zeroThreshold,
      positiveBucketOffset: p.positiveOffset,
      positiveBucketCounts: p.positiveCounts,
      negativeBucketOffset: p.negativeOffset,
      negativeBucketCounts: p.negativeCounts,
      aggregationTemporality: temporality || 'Delta',
    }
  })

  // Heatmap selection-highlight timestamp in ms. Resolves the active
  // datapoint (selected or latest fallback) to the column it lives in.
  // The heatmap will mark that column with a persistent ring so the
  // user can scan back from "I selected this snapshot" to "this is
  // when it was."
  let heatmapSelectedTimestamp = $derived.by((): number | null => {
    const dp = selectedDatapoint ?? latestHistogramDp
    if (!dp) return null
    return Number(dp.timestamp / 1_000_000n)
  })

  // -- Datapoint list state --------------------------------------------
  let expandedDatapoints = new SvelteSet<string>()

  function toggleDatapoint(id: string) {
    if (expandedDatapoints.has(id)) {
      expandedDatapoints.delete(id)
    } else {
      expandedDatapoints.add(id)
    }
  }

  // A datapoint is "expandable" if it carries auxiliary data (attributes
  // or exemplars). Otherwise the chevron + expand machinery is just
  // visual noise -- the row's primary job is to act as a chart-snapshot
  // selector.
  function isExpandable(dp: DataPoint): boolean {
    return dp.attributes.length > 0 || dp.exemplars.length > 0
  }

  // Click handler: select this row, switch to Snapshot tab on
  // histograms, and toggle the expand state for rows that have details.
  // Selection is independent of expansion so users can highlight a
  // clean row without anything visually changing inline.
  function onRowClick(dp: DataPoint) {
    selectedDatapointId = selectedDatapointId === dp.id ? null : dp.id
    if (isHistogramKind && selectedDatapointId !== null) {
      activeTab = 'snapshot'
    }
    if (isExpandable(dp)) {
      toggleDatapoint(dp.id)
    }
  }

  // Heatmap click handler: incoming timestamp is the bucket-start in ms.
  // Resolve to the FIRST datapoint in that bucket (there can be many
  // when the bucket-width compressed multiple raw timestamps together;
  // we just pick something deterministic so the snapshot view has
  // SOMETHING to render). Then switch to Snapshot tab.
  function onHeatmapSelect(timestampMs: number) {
    if (!metric) return
    // Bucket width matches what the backend used: max(1ms, (end-start)/100).
    const bucketWidthMs = Math.max(1, Math.floor((queryRange.end - queryRange.start) / 100))
    const bucketStart = BigInt(timestampMs) * 1_000_000n
    const bucketEnd = BigInt(timestampMs + bucketWidthMs) * 1_000_000n
    const found = metric.datapoints.find(
      dp => dp.timestamp >= bucketStart && dp.timestamp < bucketEnd
    )
    if (found) {
      selectedDatapointId = found.id
      activeTab = 'snapshot'
    }
  }

  function datapointValue(dp: DataPoint): string {
    if (dp.metricType === 'Gauge' || dp.metricType === 'Sum') {
      return String(dp.doubleValue ?? dp.intValue ?? '—')
    }
    if (dp.metricType === 'Histogram' || dp.metricType === 'ExponentialHistogram') {
      return `count: ${dp.count}, sum: ${dp.sum.toFixed(2)}`
    }
    return '—'
  }

  // Tab strip definition. Order matters for keyboard nav; left-to-right
  // mirrors the conceptual progression overview -> aggregate -> single
  // snapshot, which is also the typical investigative flow.
  const TABS: { id: 'heatmap' | 'aggregated' | 'snapshot'; label: string }[] = [
    { id: 'heatmap', label: 'Heatmap' },
    { id: 'aggregated', label: 'Aggregated' },
    { id: 'snapshot', label: 'Snapshot' },
  ]
</script>

{#snippet fieldsSection()}
  <!-- Field table.
       Same DOM shape as SpanField (TraceDetails). Everything OTLP
       gives us at the metric level becomes a row here -- including
       name, description, type, unit, temporality, monotonicity --
       so there's no separate "header strip" duplicating identity.
       Resource and scope attributes follow, with scope name/version
       folded in as ordinary scope rows (no field promotion). -->
  <div class="metric-detail-section">
    <table class="detail-fields w-full" aria-label="Metric fields">
      <thead class="table-header-surface metric-detail-section__head">
        <tr class="table-header-row">
          <th class="table-header-cell table-header-cell--left" colspan="2">Fields</th>
        </tr>
      </thead>
      <tbody class="table-body-surface">
        <MetricField fieldName="name" fieldValue={metric!.name} fieldType="string" />
        {#if metric!.description}
          <MetricField fieldName="description" fieldValue={metric!.description} fieldType="string" />
        {/if}
        <MetricField fieldName="type" fieldValue={metricType} fieldType="string" />
        {#if metric!.unit}
          <MetricField fieldName="unit" fieldValue={metric!.unit} fieldType="string" />
        {/if}
        {#if temporality}
          <MetricField fieldName="aggregation temporality" fieldValue={temporality} fieldType="string" />
        {/if}
        {#if isMonotonic !== null}
          <MetricField fieldName="is monotonic" fieldValue={String(isMonotonic)} fieldType="bool" />
        {/if}
        <MetricField
          fieldName="received"
          fieldValue={formatTimestamp(metric!.received, timeContext.timezone, 'milliseconds')}
          fieldType="timestamp"
        />
        <MetricField
          fieldName="datapoint count"
          fieldValue={metric!.datapoints.length.toString()}
          fieldType="uint32"
        />
        {#if metric!.resourceDroppedAttributesCount > 0}
          <MetricField
            fieldName="dropped attributes"
            fieldValue={metric!.resourceDroppedAttributesCount.toString()}
            fieldType="uint32"
            origin="resource"
          />
        {/if}
        {#each resourceAttrs as attr (`resource:${attr.key}`)}
          <MetricField fieldName={attr.key} fieldValue={attr.value} fieldType={attr.type} origin="resource" />
        {/each}
        {#each scopeAttrs as attr (`scope:${attr.key}`)}
          <MetricField fieldName={attr.key} fieldValue={attr.value} fieldType={attr.type} origin="scope" />
        {/each}
      </tbody>
    </table>
  </div>
{/snippet}

{#snippet datapointsSection()}
  <!-- Same height/scroll contract as fieldsSection: a flex-1 + min-h-0
       wrapper so the section consumes its panel's remaining space and
       scrolls internally only on overflow. Header strip mirrors the
       Fields <thead> exactly. List itself is NOT collapsible -- per-row
       expansion is the only disclosure here, and only for rows that
       actually have attributes or exemplars. -->
  <div class="metric-detail-section metric-detail-datapoints">
    <div class="table-header-surface metric-detail-section__head">
      <span class="table-header-cell table-header-cell--left">
        Datapoints ({metric!.datapoints.length})
      </span>
    </div>
    <div class="metric-detail-datapoints__list">
      {#each metric!.datapoints as dp (dp.id)}
        {@const expanded = expandedDatapoints.has(dp.id)}
        {@const expandable = isExpandable(dp)}
        {@const selected = selectedDatapointId === dp.id}
        <div class="metric-dp-row {selected ? 'metric-dp-row--selected' : ''}">
          <button
            type="button"
            class="metric-dp-row__header"
            onclick={() => onRowClick(dp)}
            aria-pressed={selected}
          >
            <span class="tabular-nums text-base-content/70">{formatTimestamp(dp.timestamp, timeContext.timezone, 'milliseconds')}</span>
            <span class="text-base-content">{datapointValue(dp)}</span>
            {#if dp.attributes.length > 0}
              <span class="badge badge-xs badge-soft badge-neutral">{dp.attributes.length} attr{dp.attributes.length !== 1 ? 's' : ''}</span>
            {/if}
            {#if expandable}
              <svg class="w-3 h-3 shrink-0 transition-transform {expanded ? 'rotate-180' : ''}" viewBox="0 0 24 24">
                <path d="M18 9s-4.419 6-6 6s-6-6-6-6" />
              </svg>
            {/if}
          </button>
          {#if expanded && expandable}
            <div class="metric-dp-row__details">
              {#each dp.attributes as attr (attr.key)}
                <div class="metric-dp-attr">
                  <span class="detail-cell__key">{attr.key}:</span>
                  <span>{attr.value}</span>
                  <span class="badge-type ml-auto">{attr.type}</span>
                </div>
              {/each}
              {#if dp.exemplars.length > 0}
                <div class="metric-dp-exemplars-header">Exemplars</div>
                {#each dp.exemplars as ex (ex.timestamp)}
                  <div class="metric-dp-attr">
                    <span class="tabular-nums text-base-content/70">{formatTimestamp(ex.timestamp, timeContext.timezone, 'milliseconds')}</span>
                    <span>value: {ex.value}</span>
                    {#if ex.traceID}
                      <a href="/trace/{ex.traceID}" class="link link-primary text-xs font-mono ml-auto">trace</a>
                    {/if}
                  </div>
                {/each}
              {/if}
            </div>
          {/if}
        </div>
      {/each}
    </div>
  </div>
{/snippet}

{#snippet bucketSeriesErrorMessage(err: BucketSeriesError)}
  <div class="metric-detail-chart__placeholder text-error/70">
    {#if err.kind === 'unspecified'}
      Aggregation temporality is Unspecified — backend can't safely combine these datapoints.
    {:else if err.kind === 'boundsMismatch'}
      Histogram bounds disagree across datapoints in this window — backend can't merge.
    {:else}
      {err.message}
    {/if}
  </div>
{/snippet}

{#snippet histogramChartSlot()}
  <!-- Three tabs share this slot. The tab strip keeps the chart frame
       a constant pixel height regardless of which view is active so
       the bottom split panels don't reflow when the user switches
       tabs. Errors and loading states are unified across tabs because
       they all derive from the same fetches (bucket series + aggregated
       point). -->
  <div class="metric-detail-chart">
    <div class="metric-detail-tabs" role="tablist" aria-label="Histogram views">
      {#each TABS as tab (tab.id)}
        <button
          type="button"
          role="tab"
          class="metric-detail-tab"
          class:metric-detail-tab--active={activeTab === tab.id}
          aria-selected={activeTab === tab.id}
          onclick={() => (activeTab = tab.id)}
        >
          {tab.label}
        </button>
      {/each}
    </div>
    <div class="metric-detail-chart__body">
      {#if activeTab === 'heatmap'}
        {#if bucketSeriesError}
          {@render bucketSeriesErrorMessage(bucketSeriesError)}
        {:else if bucketSeriesLoading || bucketSeries === null}
          <div class="metric-detail-chart__placeholder">Loading heatmap…</div>
        {:else}
          <HistogramHeatmap
            points={bucketSeries}
            windowStartMs={queryRange.start}
            windowEndMs={queryRange.end}
            height={250}
            onSelect={onHeatmapSelect}
            selectedTimestamp={heatmapSelectedTimestamp}
          />
        {/if}
      {:else if activeTab === 'aggregated'}
        {#if aggregatedError}
          {@render bucketSeriesErrorMessage(aggregatedError)}
        {:else if aggregatedLoading || !aggregatedDatapoint}
          <div class="metric-detail-chart__placeholder">Loading aggregate…</div>
        {:else}
          <HistogramChart
            datapoint={aggregatedDatapoint}
            unit={metric!.unit}
            quantileSource="aggregated"
            metricID={metric!.id}
            windowStartMs={queryRange.start}
            windowEndMs={queryRange.end}
          />
        {/if}
      {:else if activeTab === 'snapshot'}
        {#if activeHistogramDp}
          <div class="metric-detail-chart__subtitle">
            <span class="text-base-content/55 text-xs">datapoint at</span>
            <span class="text-base-content text-xs tabular-nums">
              {formatTimestamp(activeHistogramDp.timestamp, timeContext.timezone, 'milliseconds')}
            </span>
          </div>
          <HistogramChart
            datapoint={activeHistogramDp}
            unit={metric!.unit}
            quantileSource="datapoint"
          />
        {:else}
          <div class="metric-detail-chart__placeholder">No datapoint selected</div>
        {/if}
      {/if}
    </div>
  </div>
{/snippet}

{#snippet timeSeriesChartSlot()}
  <!-- Section label mirrors the tab strip's vertical position so the
       chart row's pixel height is the same for Gauge/Sum as for
       histograms. Keeps the bottom split panels from reflowing when
       the user navigates between metric types. -->
  <div class="metric-detail-chart">
    <div class="metric-detail-section-label">Time series</div>
    <div class="metric-detail-chart__body">
      <MetricTimeSeriesChart datapoints={metric!.datapoints} {highlightedTimestamp} />
    </div>
  </div>
{/snippet}

{#snippet panelFooter()}
  <button
    type="button"
    class="btn btn-ghost btn-sm text-error"
    onclick={() => onDelete?.(metric!.id)}
    aria-label="Delete this metric"
    disabled={!onDelete}
  >
    <TrashIcon class="h-3.5 w-3.5" aria-hidden="true" />
    Delete this metric
  </button>
  <span aria-hidden="true"></span>
  <DetailNav
    {index}
    {total}
    label="metric"
    onFirst={() => onFirst?.()}
    onPrev={() => onPrev?.()}
    onNext={() => onNext?.()}
    onLast={() => onLast?.()}
  />
{/snippet}

{#if metric}
  <!--
    Two-row layout, unified across all metric types:
      Row 1 (chart): tab strip OR section label + chart for the metric.
      Row 2 (split): Fields | Datapoints in a ResizablePanels with
                     independent scroll per pane.
      Row 3 (footer): delete + DetailNav, anchored.
    Outer chrome (border + frosted bg + rounded + shadow) lives on
    .metric-detail-panel; the inner rows are flat sections separated
    only by hairline dividers, so the whole thing reads as one panel
    instead of nested boxes.
  -->
  <div class="metric-detail-panel">
    {#if isUnspecifiedTemporality}
      <!-- FunError takes the entire chart row. Bottom split + footer
           still render so the user can see Fields, Datapoints, and
           navigate -- the metric's instrumentation is broken but the
           data we DO have is still useful for diagnosis. -->
      <div class="metric-detail-chart">
        <UnspecifiedTemporalityCallout size="full" />
      </div>
    {:else if isHistogramKind}
      {@render histogramChartSlot()}
    {:else if metricType === 'Gauge' || metricType === 'Sum'}
      {@render timeSeriesChartSlot()}
    {:else}
      <div class="metric-detail-chart">
        <div class="metric-detail-chart__placeholder">
          No chart available for this metric type
        </div>
      </div>
    {/if}

    <div class="metric-detail-panel__split">
      <ResizablePanels
        defaultLeftWidth={0.5}
        minLeftWidth={0.3}
        minRightWidth={0.3}
        storageKey="metric-detail-panels"
      >
        {#snippet leftPanel()}
          <div class="metric-detail-pane">{@render fieldsSection()}</div>
        {/snippet}
        {#snippet rightPanel()}
          <div class="metric-detail-pane">{@render datapointsSection()}</div>
        {/snippet}
      </ResizablePanels>
    </div>

    <div class="metric-detail-panel__footer">
      {@render panelFooter()}
    </div>
  </div>
{:else}
  <div class="metric-detail-panel metric-detail-panel--empty">
    <p class="text-base-content/40 text-sm">Select a metric to view details</p>
  </div>
{/if}

<style lang="postcss">
  @reference "../../app.css";

  /*
   * Outer panel. Carries the chrome (border + frosted bg + rounded +
   * shadow + backdrop-blur) so the whole metric detail reads as one
   * unified surface. Inner rows are flat sections separated by hairline
   * dividers; chrome at the top level only.
   *
   * Three-row flex column: chart row (intrinsic), split row (flex-1
   * with min-h-0 so it can shrink past its content), footer (intrinsic).
   */
  .metric-detail-panel {
    @apply flex h-full min-h-0 w-full flex-col overflow-hidden rounded-xl border border-base-300/70 bg-base-100/80 shadow-surface-sm backdrop-blur-sm;
  }

  .metric-detail-panel--empty {
    @apply h-full items-center justify-center;
  }

  /*
   * Chart row. shrink-0 so the chart keeps its requested height
   * regardless of how big the bottom split is. Bottom border separates
   * it from the Fields/Datapoints split below.
   */
  .metric-detail-chart {
    @apply shrink-0 px-2 py-3 border-b border-base-300/30;
  }

  .metric-detail-chart__body {
    @apply mt-2;
  }

  .metric-detail-chart__placeholder {
    @apply flex items-center justify-center text-base-content/40 text-sm;
    height: 250px;
  }

  /*
   * Inline subtitle for the Snapshot tab. Sits right under the tab
   * strip and tells the user WHICH datapoint they're looking at --
   * critical context now that the chart only shows one snapshot at a
   * time. Spaced compactly so it doesn't push the chart down too far.
   */
  .metric-detail-chart__subtitle {
    @apply mt-1 mb-1 flex items-baseline gap-2 px-2;
  }

  /*
   * Underline-style tabs. Inactive tabs get muted text + transparent
   * underline so the active one's primary-coloured underline
   * provides the only visual contrast. Hover lifts inactive tabs
   * partially so the affordance is obvious without competing for
   * attention.
   */
  .metric-detail-tabs {
    @apply flex items-center gap-1 border-b border-base-300/40 px-2;
  }

  .metric-detail-tab {
    @apply px-3 py-1.5 text-xs font-medium text-base-content/55 border-b-2 border-transparent transition-colors;
  }

  .metric-detail-tab:hover {
    @apply text-base-content/80;
  }

  .metric-detail-tab--active {
    @apply text-base-content;
    border-bottom-color: var(--color-primary);
  }

  /*
   * Section label for Gauge/Sum chart row. Visually anchored to the
   * same baseline as the tab strip so swapping metric types doesn't
   * shift the chart's vertical position. Muted and small to read as
   * "section heading" rather than "click me".
   */
  .metric-detail-section-label {
    @apply px-2 py-1.5 text-xs font-medium text-base-content/55 border-b border-base-300/40;
  }

  /*
   * Bottom split row. flex-1 + min-h-0 is the magic that lets
   * ResizablePanels actually fill remaining space without forcing the
   * chart row off-screen. Without min-h-0 the split would push past
   * the panel's height and the footer would be unreachable.
   */
  .metric-detail-panel__split {
    @apply flex min-h-0 flex-1;
  }

  /*
   * Per-pane container inside the split. Flat (no chrome) since the
   * outer .metric-detail-panel owns it. min-h-0 + overflow-hidden lets
   * each pane's section scroll independently rather than the split
   * itself scrolling as one unit.
   */
  .metric-detail-pane {
    @apply flex h-full min-h-0 flex-col overflow-hidden;
  }

  .metric-detail-panel__footer {
    @apply flex items-center justify-between gap-2 border-t border-base-300/50 px-4 py-2;
  }

  /*
   * Shared section shell for Fields and Datapoints. Each pane is a
   * single-section flex-col with the section flex-1, so the section
   * consumes the pane's full height and scrolls its own content on
   * overflow. min-h-0 lets it actually shrink past its intrinsic
   * size so the scroll engages.
   */
  .metric-detail-section {
    @apply flex min-h-0 flex-1 flex-col overflow-y-auto;
  }

  /*
   * Sticky section head -- when the section's body scrolls, the
   * "Fields" / "Datapoints" strip stays anchored at the top so the
   * user keeps a sense of place.
   */
  .metric-detail-section__head {
    @apply sticky top-0 z-10;
  }

  .metric-dp-row {
    @apply border-b border-base-300/20;
  }

  /* Selected row mirrors the SignalCard selection treatment: a tinted
     background plus a primary-colour spine so the user can scan-locate
     the chart-anchored row even after scrolling the list. */
  .metric-dp-row--selected {
    @apply bg-primary/[0.07];
    box-shadow: inset 3px 0 0 0 var(--color-primary);
  }

  .metric-dp-row__header {
    @apply flex w-full items-center gap-3 px-4 py-1.5 text-xs text-left hover:bg-base-200/50 transition-colors;
  }

  .metric-dp-row__details {
    @apply px-6 pb-2 space-y-0.5;
  }

  .metric-dp-attr {
    @apply flex items-center gap-2 text-xs py-0.5;
  }

  .metric-dp-exemplars-header {
    @apply text-xs font-semibold text-base-content/55 pt-1;
  }
</style>

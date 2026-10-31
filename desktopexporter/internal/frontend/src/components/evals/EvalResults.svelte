<script lang="ts">
  import { createLoadingNotice } from '@/components/shared/utils/loading-notice.svelte'
  import LoadingStatus from '@/components/shared/LoadingStatus.svelte'
  import { untrack } from 'svelte'
  import type { EvalRequestPart } from './eval-request-state'
  import type { Snippet } from 'svelte'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import {
    CheckListIcon,
    ToolsIcon,
    Coins01Icon,
  } from '@hugeicons/core-free-icons'
  import PaneHeader from '@/components/shared/WorkspaceHeader.svelte'
  import Identifier from '@/components/shared/Identifier.svelte'
  import AssessmentBadge from './AssessmentBadge.svelte'
  import ExperimentStatusBadge from './ExperimentStatusBadge.svelte'
  import EvalStatus from './EvalStatus.svelte'
  import { caseSummary } from './case-summary'
  import TableSortButton from '@/components/shared/TableSortButton.svelte'
  import TableColumnHeading from '@/components/shared/TableColumnHeading.svelte'
  import ResizableTable from '@/components/shared/ResizableTable.svelte'
  import TreeRowIdentity from '@/components/shared/TreeRowIdentity.svelte'
  import DurationBar from '@/components/shared/DurationBar.svelte'
  import TimingCell from '@/components/shared/TimingCell.svelte'
  import EvalTimelineMarkers from './EvalTimelineMarkers.svelte'
  import { evalTimelineRecords } from './eval-timeline'
  import { uniqueToolCallID, toolLogOwners } from './eval-span-evidence'
  import type { TimelineRecord } from '@/components/traces/Waterfall/timeline-markers'
  import { navigate } from '@/route'
  import { telemetryAPI } from '@/services/telemetry-service'
  import type {
    LogData,
    SpanData,
    SpanTiming,
    TraceData,
    TraceLogSummary,
  } from '@/types/api-types'
  import { getServiceName } from '@/utils/resource'
  import {
    evalExecutionRows,
    evalTraceReferences,
    indexEvalTraces,
    referenceKey,
    resolveEvalReference,
    projectEvalTraceEvidence,
    resultTraceNodes,
  } from './eval-traces'
  import {
    buildEvalOutline,
    combineEstimates,
    outlineOpen,
    visibleOutlineRows,
    type EvalOutlineRow,
    type TimeWindow,
  } from './eval-outline'
  import { formatDuration } from '@/utils/time'
  import { spanTimingColor } from '@/utils/trace-service-color'
  import {
    buildWaterfallRows,
    getTraceBounds,
  } from '@/components/traces/Waterfall/WaterfallView.svelte'
  import { themeSignal } from '@/state/theme.svelte'
  import {
    completionLabel,
    completionTone,
    groupResults,
  } from './group-results'
  import {
    estimatedCost,
    type TelemetryReference,
    type AssessmentDefinition,
    type EvalResult,
    type EvalExperiment,
    type EvalView,
  } from './model'
  import { type ResultSort, type ResultSortKey } from './sort-results'

  let {
    experiment: sourceExperiment,
    results,
    selectedID,
    selectedRowID,
    onSelect,
    onResolveSelection,
    onTraceViews,
    viewKey = $bindable('results'),
    headerActions,
    beforeTable,
    loadingStatus,
    disclosure = $bindable({}),
    sorts = $bindable({}),
  }: {
    experiment: EvalExperiment
    results: EvalResult[]
    selectedID?: string
    selectedRowID?: string
    onSelect?: (row: EvalOutlineRow) => void
    onResolveSelection?: (row: EvalOutlineRow | undefined) => void
    onTraceViews?: (
      traces: TraceData[],
      loading: boolean,
      logs: LogData[]
    ) => void
    viewKey?: EvalView
    headerActions?: Snippet
    beforeTable?: Snippet
    loadingStatus?: Snippet
    disclosure?: Record<string, Record<string, boolean>>
    sorts?: Record<string, ResultSort | undefined>
  } = $props()
  // Retained telemetry belongs to its experiment, not to the mounted pane.
  // In particular, another experiment's bounds must not become this one's axis.
  let displayedTraces = $state.raw<{
    experimentID: string
    traces: TraceData[]
  }>()
  const traceViews = $derived(
    displayedTraces?.experimentID === sourceExperiment.id
      ? displayedTraces.traces
      : []
  )
  let displayedLogs = $state.raw<{
    experimentID: string
    owners: Map<string, LogData[]>
  }>()
  let displayedTimeline = $state.raw<{
    experimentID: string
    timeline: Map<string, TraceLogSummary[]>
  }>()
  const correlationLogs = $derived(
    displayedLogs?.experimentID === sourceExperiment.id
      ? [...displayedLogs.owners]
          .sort(([a], [b]) => a.localeCompare(b))
          .flatMap(([, logs]) => logs)
      : []
  )
  const traceIndex = $derived(indexEvalTraces(traceViews))
  const experiment = $derived(
    projectEvalTraceEvidence(sourceExperiment, traceIndex, correlationLogs)
  )
  const panelID = $props.id()
  const views: { id: EvalView; label: string; icon: typeof CheckListIcon }[] = [
    { id: 'results', label: 'Results', icon: CheckListIcon },
    { id: 'time', label: 'Tool calls', icon: ToolsIcon },
    { id: 'cost', label: 'Cost', icon: Coins01Icon },
  ]
  const enabledViews = $derived([viewKey])
  const viewLabel = $derived(views.find(item => item.id === viewKey)!.label)
  function selectView(event: KeyboardEvent, index: number) {
    const direction = ['ArrowRight', 'ArrowDown'].includes(event.key)
      ? 1
      : ['ArrowLeft', 'ArrowUp'].includes(event.key)
        ? -1
        : 0
    if (!direction) return
    event.preventDefault()
    let next = index
    do {
      next = (next + direction + views.length) % views.length
    } while (!availableViews[views[next].id])
    viewKey = views[next].id
    const button = event.currentTarget
    if (!(button instanceof HTMLButtonElement)) return
    button.parentElement
      ?.querySelectorAll<HTMLButtonElement>('[role="radio"]')
      [next]?.focus()
  }
  let caseGutterWidth = $state(0)
  let resultGutterWidth = $state(0)
  const countDigits = $derived.by(() => {
    const repetitions = new Map<string, number>()
    let maximum = Math.max(
      1,
      new Set(experiment.results.map(item => item.caseID)).size
    )
    for (const result of experiment.results) {
      const count = (repetitions.get(result.caseID) ?? 0) + 1
      repetitions.set(result.caseID, count)
      maximum = Math.max(
        maximum,
        count,
        result.tools.length +
          result.assessments.filter(item => item.telemetry?.spanID).length
      )
    }
    return String(maximum).length
  })
  const overrides = $derived(disclosure[experiment.id] ?? {})
  function toggleOutline(row: EvalOutlineRow) {
    disclosure = {
      ...disclosure,
      [experiment.id]: {
        ...overrides,
        [row.id]: !outlineOpen(row, enabledViews, overrides),
      },
    }
  }
  const spanColors = $derived(
    Object.fromEntries(
      traceViews.flatMap(trace =>
        buildWaterfallRows(
          trace.spans,
          getTraceBounds(trace.spans),
          themeSignal.value
        ).map(row => [referenceKey(row.spanNode.spanData), row.color])
      )
    )
  )
  const spanTimings = $derived<Record<string, SpanTiming>>(
    Object.fromEntries(
      [...traceIndex.nodes].map(([key, node]) => [
        key,
        {
          ...node.spanData,
          serviceName: getServiceName(node.spanData.resource) ?? '',
        },
      ])
    )
  )
  const execution = $derived(
    Object.fromEntries(
      experiment.results.map(result => [
        result.id,
        evalExecutionRows(result, traceIndex),
      ])
    )
  )
  let timingsLoading = $state(false)
  let timingRequest = $state.raw<AbortSignal>()
  const timingNotice = createLoadingNotice(
    () => timingsLoading,
    () => timingRequest
  )
  let timingsFailed = $state(false)
  const timelineLogs = $derived(
    displayedTimeline?.experimentID === sourceExperiment.id
      ? displayedTimeline.timeline
      : new Map<string, TraceLogSummary[]>()
  )
  let activityProgress = $state.raw(
    new Map<string, { pending: boolean; failed: boolean }>()
  )
  const activityLoading = $derived(
    traceViews.some(
      trace =>
        activityProgress.get(trace.traceID.toLowerCase())?.pending ?? true
    )
  )
  const activityFailed = $derived(
    [...activityProgress.values()].some(part => part.failed)
  )
  let activityRequest = $state.raw<AbortSignal>()
  function loadTraceActivity(
    trace: TraceData,
    source: EvalExperiment,
    controller: AbortController
  ) {
    const experimentID = source.id
    const traceID = trace.traceID.toLowerCase()
    const prefix = `${traceID}/`
    const nodes = indexEvalTraces([trace]).nodes
    const traces = [trace]
    const summaries = new Map<string, EvalRequestPart<TraceLogSummary[]>>(
      traces.map(trace => [trace.traceID.toLowerCase(), { status: 'pending' }])
    )
    const owners = new Map<string, EvalRequestPart<LogData[]>>()
    const previousLogs = untrack(() => displayedLogs)
    const received = new Map(
      previousLogs?.experimentID === experimentID
        ? [...previousLogs.owners].filter(([key]) => key.startsWith(prefix))
        : []
    )
    // Successful summaries replace only their own trace's discovery scope.
    // Explicit span references and native call ancestry authorize owners separately.
    type OwnerReference = Pick<SpanData, 'traceID' | 'spanID'>
    const independentOwners = new Map<string, OwnerReference>()
    for (const reference of evalTraceReferences(source)) {
      if (!reference.spanID || reference.traceID.toLowerCase() !== traceID)
        continue
      const key = referenceKey(reference)
      independentOwners.set(key, {
        traceID: reference.traceID,
        spanID: reference.spanID,
      })
    }
    const nativeOwners = new Map<string, SpanData>()
    for (const { spanData: span } of nodes.values()) {
      const callID = uniqueToolCallID(span, nodes)
      if (!callID) continue
      for (const owner of toolLogOwners([span], nodes, callID).slice(1)) {
        const key = referenceKey(owner)
        nativeOwners.set(key, owner)
        independentOwners.set(key, owner)
      }
    }
    const summaryOwners = new Map<string, Set<string>>()
    const ownerControllers = new Map<string, AbortController>()
    function publishProgress() {
      if (controller.signal.aborted) return
      const parts = [...summaries.values(), ...owners.values()]
      activityProgress = new Map(untrack(() => activityProgress)).set(traceID, {
        pending: parts.some(part => part.status === 'pending'),
        failed: parts.some(part => part.status === 'failed'),
      })
    }
    function publishLogs() {
      if (controller.signal.aborted) return
      const previous = untrack(() => displayedLogs)
      const siblings =
        previous?.experimentID === experimentID
          ? [...previous.owners].filter(([key]) => !key.startsWith(prefix))
          : []
      displayedLogs = {
        experimentID,
        owners: new Map([...siblings, ...received]),
      }
    }
    function authorized(span: OwnerReference) {
      const key = referenceKey(span)
      const discovered = summaryOwners.get(span.traceID.toLowerCase())
      return independentOwners.has(key) || !discovered || discovered.has(key)
    }
    function replaceSummaryOwners(traceID: string, discovered: Set<string>) {
      summaryOwners.set(traceID, discovered)
      for (const key of received.keys()) {
        if (
          key.startsWith(`${traceID}/`) &&
          !independentOwners.has(key) &&
          !discovered.has(key)
        )
          received.delete(key)
      }
      for (const [key, ownerController] of ownerControllers) {
        if (
          key.startsWith(`${traceID}/`) &&
          !independentOwners.has(key) &&
          !discovered.has(key)
        ) {
          ownerController.abort()
          owners.delete(key)
        }
      }
      publishLogs()
    }
    function requestOwner(span: OwnerReference) {
      const key = referenceKey(span)
      if (owners.has(key) || controller.signal.aborted || !authorized(span))
        return
      const ownerController = new AbortController()
      ownerControllers.set(key, ownerController)
      owners.set(key, { status: 'pending' })
      publishProgress()
      void telemetryAPI
        .getSpanLogs(span.traceID, span.spanID, ownerController.signal)
        .then(
          logs => {
            if (
              controller.signal.aborted ||
              ownerController.signal.aborted ||
              !authorized(span)
            )
              return
            received.set(key, logs)
            owners.set(key, { status: 'ready', value: logs })
            publishLogs()
            publishProgress()
          },
          () => {
            if (
              controller.signal.aborted ||
              ownerController.signal.aborted ||
              !authorized(span)
            )
              return
            owners.set(key, { status: 'failed' })
            publishProgress()
          }
        )
    }
    publishProgress()
    for (const owner of nativeOwners.values()) requestOwner(owner)
    // Refresh known explicit owners even if summary discovery no longer lists them.
    for (const key of received.keys()) {
      const owner = independentOwners.get(key)
      if (owner) requestOwner(owner)
    }
    const traceIDs = new Set(traces.map(trace => trace.traceID.toLowerCase()))
    for (const key of received.keys())
      if (!traceIDs.has(key.split('/')[0]!) && !independentOwners.has(key))
        received.delete(key)
    publishLogs()
    for (const trace of traces) {
      void telemetryAPI
        .getTraceLogSummaries(trace.traceID, controller.signal)
        .then(
          logs => {
            if (controller.signal.aborted) return
            const id = trace.traceID.toLowerCase()
            summaries.set(id, { status: 'ready', value: logs })
            const previous = untrack(() => displayedTimeline)
            const timeline = new Map(
              previous?.experimentID === experimentID ? previous.timeline : []
            )
            timeline.set(id, logs)
            displayedTimeline = { experimentID, timeline }
            const discovered = new Map<string, SpanData>()
            for (const log of logs) {
              if (
                !log.spanID ||
                !['tool.execution', 'tool.execution.start'].includes(
                  log.eventName
                )
              )
                continue
              const owner = nodes.get(
                referenceKey({ traceID: id, spanID: log.spanID })
              )?.spanData
              if (owner) discovered.set(referenceKey(owner), owner)
            }
            replaceSummaryOwners(id, new Set(discovered.keys()))
            for (const owner of discovered.values()) requestOwner(owner)
            publishProgress()
          },
          () => {
            summaries.set(trace.traceID.toLowerCase(), { status: 'failed' })
            publishProgress()
          }
        )
    }
    controller.signal.addEventListener(
      'abort',
      () => {
        for (const ownerController of ownerControllers.values())
          ownerController.abort()
      },
      { once: true }
    )
  }
  function selectActivity(record: TimelineRecord, span: SpanTiming) {
    const params = new URLSearchParams({ span: span.spanID })
    if (record.kind === 'event') params.set('event', String(record.eventIndex))
    else params.set('log', record.log.logRef)
    navigate(`/traces/${span.traceID}?${params}`)
  }
  $effect(() => onTraceViews?.(traceViews, timingsLoading, correlationLogs))
  const timingKey = referenceKey
  const timingFor = (reference?: TelemetryReference) => {
    const node = resolveEvalReference(reference, traceIndex)
    return node ? spanTimings[timingKey(node.spanData)] : undefined
  }
  function durationFor(reference?: TelemetryReference, seconds?: number) {
    const span = timingFor(reference)
    if (span && span.endTime >= span.startTime)
      return span.endTime - span.startTime
    if (seconds === undefined || seconds < 0 || !Number.isFinite(seconds * 1e9))
      return undefined
    return BigInt(Math.round(seconds * 1e9))
  }
  function missingTiming(reference?: TelemetryReference) {
    if (!reference?.spanID) return 'No recorded duration'
    if (timingsLoading) return timingNotice.visible ? 'Loading span' : ''
    if (timingsFailed) return 'Span unavailable'
    return timingFor(reference) ? 'Invalid span duration' : 'Span not stored'
  }
  function spanWindow(spans: (SpanTiming | undefined)[]) {
    let window: { startTime: bigint; endTime: bigint } | undefined
    for (const span of spans) {
      if (!span || span.endTime < span.startTime) continue
      if (!window) window = { startTime: span.startTime, endTime: span.endTime }
      else {
        if (span.startTime < window.startTime) window.startTime = span.startTime
        if (span.endTime > window.endTime) window.endTime = span.endTime
      }
    }
    return window
  }
  const resultWindows = $derived(
    Object.fromEntries(
      experiment.results.map(result => [
        result.id,
        spanWindow([
          timingFor(result.telemetry),
          timingFor(result.resultTelemetry),
          ...resultTraceNodes(result, traceIndex).map(node =>
            timingFor(node.spanData)
          ),
          ...result.tools.map(tool => timingFor(tool.telemetry)),
          ...result.assessments.map(assessment =>
            timingFor(assessment.telemetry)
          ),
        ]),
      ])
    )
  )
  const experimentWindow = $derived(spanWindow(Object.values(spanTimings)))
  const maximumDuration = $derived.by(() => {
    const extent = experimentWindow
      ? experimentWindow.endTime - experimentWindow.startTime
      : 0n
    return [
      experiment,
      ...experiment.results.flatMap(result => [result, ...result.tools]),
    ].reduce((maximum, item) => {
      const duration = durationFor(item.telemetry, item.seconds) ?? 0n
      return duration > maximum ? duration : maximum
    }, extent)
  })
  $effect(() => {
    const source = sourceExperiment
    const experimentID = source.id
    const references = evalTraceReferences(source)
    const ids = [
      ...new Set(references.map(reference => reference.traceID.toLowerCase())),
    ]
    const controller = new AbortController()
    timingRequest = controller.signal
    activityRequest = controller.signal
    activityProgress = new Map()
    timingsLoading = true
    timingsFailed = false
    const previous = untrack(() => displayedTraces)
    const received = new Map(
      (previous?.experimentID === experimentID ? previous.traces : [])
        .filter(trace => ids.includes(trace.traceID.toLowerCase()))
        .map(trace => [trace.traceID.toLowerCase(), trace])
    )
    const completed = new Set<string>()
    function publishTrace(trace: TraceData) {
      if (controller.signal.aborted) return
      const id = trace.traceID.toLowerCase()
      if (completed.has(id)) return
      completed.add(id)
      received.set(id, trace)
      displayedTraces = {
        experimentID,
        traces: ids.flatMap(id =>
          received.has(id) ? [received.get(id)!] : []
        ),
      }
      loadTraceActivity(trace, source, controller)
    }
    function failedTrace(id: string) {
      if (controller.signal.aborted) return
      timingsFailed = true
      activityProgress = new Map(untrack(() => activityProgress)).set(id, {
        pending: false,
        failed: true,
      })
    }
    void telemetryAPI
      .getReferencedTraceViews(references, controller.signal, result => {
        if (result.status === 'ready') publishTrace(result.trace)
        else failedTrace(result.traceID)
      })
      .then(
        result => {
          if (controller.signal.aborted) return
          for (const trace of result.traces) publishTrace(trace)
          for (const id of result.unavailableTraceIDs) failedTrace(id)
          if (!ids.length) displayedTraces = { experimentID, traces: [] }
          timingsFailed = result.unavailableTraceIDs.length > 0
          timingsLoading = false
        },
        () => {
          if (controller.signal.aborted) return
          timingsLoading = false
          timingsFailed = true
          for (const id of ids) failedTrace(id)
        }
      )
    return () => controller.abort()
  })
  type Column = {
    key: ResultSortKey
    label: string
    group: string
    width: number
    min: number
    fixed?: boolean
    numeric?: boolean
    description?: string
    hint?: string
    assessment?: AssessmentDefinition
  }
  const headline = $derived(
    experiment.assessments.find(
      item => item.id === experiment.headlineAssessment
    ) ?? experiment.assessments[0]
  )
  const viewColumns: Column[] = $derived(
    viewKey === 'time'
      ? [
          {
            key: 'tools',
            label: 'Tool calls',
            group: 'Tool calls',
            width: 176,
            min: 144,
          },
        ]
      : viewKey === 'results'
        ? [
            {
              key: headline ? `assessment:${headline.id}` : 'state',
              label: 'Assessment',
              group: 'Results',
              width: 160,
              min: 144,
              hint: headline?.scale,
              assessment: headline,
            },
          ]
        : [
            {
              key: 'investigatorCost',
              label: 'Investigator',
              group: 'Cost',
              width: 160,
              min: 144,
              description: 'API estimate · CAD',
            },
            {
              key: 'graderCost',
              label: 'Grader',
              group: 'Cost',
              width: 160,
              min: 144,
              description: 'API estimate · CAD',
            },
            {
              key: 'cost',
              label: 'Total cost',
              group: 'Cost',
              width: 160,
              min: 144,
              description: 'API estimate · CAD',
            },
          ]
  )
  const columns = $derived<Column[]>([
    {
      key: 'name',
      label: 'Test case',
      group: 'Test case',
      width: Math.max(1, caseGutterWidth),
      min: Math.max(1, caseGutterWidth),
      fixed: true,
    },
    {
      key: 'repetition',
      label: 'Result',
      hint: 'Recorded result',
      group: 'Test case',
      width: Math.max(320, resultGutterWidth),
      min: Math.max(144, resultGutterWidth),
      numeric: true,
    },
    {
      key: 'seconds',
      label: 'Duration',
      hint: experimentWindow
        ? 'Recorded starts and durations on one timeline across this experiment'
        : 'Recorded durations; start offsets unavailable',
      group: 'Duration',
      width: 288,
      min: maximumDuration > 1n ? 224 : 0,
    },
    ...viewColumns,
  ])
  const sortScope = $derived(JSON.stringify([experiment.id, viewKey]))
  const activeSort = $derived(sorts[sortScope])
  const measuredResults = $derived(
    experiment.results.map(result => {
      const window = resultWindows[result.id]
      const duration =
        durationFor(result.resultTelemetry) ??
        (window
          ? window.endTime - window.startTime
          : durationFor(result.telemetry, result.seconds))
      return {
        ...result,
        state: execution[result.id]?.state ?? result.state,
        seconds: duration !== undefined ? Number(duration) / 1e9 : undefined,
        startedTime: window?.startTime ?? result.startedTime,
      }
    })
  )
  const groups = $derived(
    groupResults(
      measuredResults.filter(item =>
        results.some(result => result.id === item.id)
      ),
      measuredResults,
      activeSort
    ).map(group => ({
      ...group,
      internalFault: group.allResults.some(
        item => execution[item.id]?.internalFault
      ),
      summary: caseSummary(group.allResults, experiment.assessments),
    }))
  )

  function toggleSort(key: ResultSortKey) {
    sorts[sortScope] = {
      key,
      direction:
        activeSort?.key === key && activeSort.direction === 'asc'
          ? 'desc'
          : 'asc',
    }
  }
  const estimates = $derived([
    { key: 'investigatorCost', ...estimatedCost(results, 'investigatorCost') },
    { key: 'graderCost', ...estimatedCost(results, 'graderCost') },
  ])
  const experimentCosts = $derived({
    investigatorCost: estimatedCost(experiment.results, 'investigatorCost'),
    graderCost: estimatedCost(experiment.results, 'graderCost'),
  })
  const outline = $derived(buildEvalOutline(groups, experiment, traceIndex))
  const selectionOutline = $derived(
    results.length === experiment.results.length
      ? outline
      : buildEvalOutline(
          groupResults(measuredResults, measuredResults),
          experiment,
          traceIndex
        )
  )
  const availableViews = $derived<Record<EvalView, boolean>>({
    results: true,
    time:
      experiment.results.some(result => result.toolsKnown === true) ||
      selectionOutline.some(row => row.kind === 'tool') ||
      (evalTraceReferences(sourceExperiment).length > 0 &&
        (timingsLoading || timingsFailed || activityLoading || activityFailed)),
    cost: selectionOutline.some(row => row.estimate.total !== null),
  })
  $effect(() => {
    if (!availableViews[viewKey]) viewKey = 'results'
  })
  const visibleRows = $derived(
    visibleOutlineRows(outline, enabledViews, overrides)
  )
  function selectRow(event: MouseEvent, row: EvalOutlineRow) {
    if (event.detail !== 0 && window.getSelection()?.isCollapsed === false)
      return
    const control =
      event.target instanceof Element
        ? event.target.closest('button, a, input, select, textarea')
        : null
    if (control && control !== event.currentTarget) return
    onSelect?.(row)
  }
  function selectBar(event: MouseEvent) {
    const bar = event.currentTarget
    if (!(bar instanceof HTMLElement)) return
    const id = bar.closest<HTMLElement>('[data-outline-id]')?.dataset.outlineId
    const row = outline.find(item => item.id === id)
    if (row) selectRow(event, row)
  }
  $effect(() => {
    onResolveSelection?.(
      selectionOutline.find(
        row =>
          row.id ===
          (selectedRowID ??
            (selectedID
              ? `result/${selectedID}`
              : `experiment/${experiment.id}`))
      )
    )
  })
</script>

{#snippet rowCost(estimate: { total: number | null; incomplete: boolean })}
  <span
    class="text-sm tabular-nums whitespace-nowrap"
    title={estimate.incomplete && estimate.total !== null
      ? 'Partial estimate · CAD'
      : 'Recorded estimate · CAD'}
  >
    {estimate.total === null ? 'Unknown' : estimate.total.toFixed(3)}
  </span>
{/snippet}

{#snippet toolCount(total: number, failed: number)}
  {#if total || failed}
    <span class="inline-flex items-center gap-1 whitespace-nowrap">
      {#if total}<span>{total} {total === 1 ? 'tool' : 'tools'}</span>{/if}
      {#if total && failed}<span aria-hidden="true">·</span>{/if}
      {#if failed}<span>{failed} failed</span>{/if}
    </span>
  {/if}
{/snippet}

{#snippet ownedToolCount(row: EvalOutlineRow)}
  {@render toolCount(row.toolCounts?.total ?? 0, row.toolCounts?.failed ?? 0)}
{/snippet}

{#snippet executionNode(
  reference: TelemetryReference | undefined,
  state: EvalOutlineRow['state'],
  internalFault = false
)}
  {@const span = timingFor(reference)}
  {#if state === 'Error'}
    <EvalStatus
      label={completionLabel(state, internalFault)}
      dotOnly
      tone={completionTone(state, internalFault)}
    />
  {:else if span}
    <span
      class="size-2 shrink-0 rounded-full"
      style:background-color={spanColors[referenceKey(span)] ??
        spanTimingColor(span, themeSignal.value)}
      role="img"
      aria-label={state}
      title={`${span.serviceName} · ${state}`}
    ></span>
  {:else}
    <span
      class="relative z-[1] size-2 shrink-0 rounded-full bg-primary"
      role="img"
      aria-label={reference
        ? 'Referenced span unavailable'
        : 'No correlated span'}
      title={reference
        ? 'Referenced span unavailable'
        : 'No telemetry reference recorded'}
    ></span>
  {/if}
{/snippet}

{#snippet spanBar(
  reference?: TelemetryReference,
  origin?: bigint,
  seconds?: number,
  spanError = false,
  window?: TimeWindow,
  label = 'Referenced execution window',
  planned = false,
  row?: EvalOutlineRow
)}
  {@const span = timingFor(reference)}
  {@const duration = durationFor(reference, seconds)}
  <TimingCell maximum={maximumDuration}>
    {#if window && window.endTime - window.startTime > 1n && (!span || span.endTime < span.startTime)}<button
        type="button"
        onclick={selectBar}
        class="eval-results__span-bar block"
        aria-label={`${label}, ${formatDuration(window.endTime - window.startTime)}`}
        title={`${label} · Timing fallback: elapsed time across available references · Start offset ${formatDuration(window.startTime - (origin ?? window.startTime))}`}
        style:--bar-color={spanError
          ? 'var(--color-error)'
          : 'var(--color-primary)'}
        ><DurationBar
          duration={window.endTime - window.startTime}
          maximum={maximumDuration}
          offset={window.startTime - (origin ?? window.startTime)}
        /></button
      >
    {:else if span && span.endTime - span.startTime > 1n}<button
        type="button"
        onclick={selectBar}
        class="eval-results__span-bar block"
        aria-label={`Span ${span.name}, ${formatDuration(span.endTime - span.startTime)}`}
        title={`${span.name} · ${span.serviceName} · ${formatDuration(span.endTime - span.startTime)} · Start offset ${formatDuration(span.startTime - (origin ?? span.startTime))}`}
        style:--bar-color={spanError
          ? 'var(--color-error)'
          : (spanColors[referenceKey(span)] ??
            spanTimingColor(span, themeSignal.value))}
        ><DurationBar
          duration={span.endTime - span.startTime}
          maximum={maximumDuration}
          offset={span.startTime - (origin ?? span.startTime)}
        /></button
      >
    {:else if duration !== undefined && duration > 1n}
      <button
        type="button"
        onclick={selectBar}
        class="block w-full text-left"
        title="Recorded eval duration"
        aria-label={`${label}, ${formatDuration(duration)} · Recorded duration; start offset unavailable`}
        style:--bar-color={spanError
          ? 'var(--color-error)'
          : 'var(--color-primary)'}
      >
        <DurationBar {duration} maximum={maximumDuration} />
      </button>
    {:else}
      <span
        class="block min-h-[var(--signal-card-row-h)]"
        title={planned ? 'Not started' : missingTiming(reference)}
        aria-busy={!!reference?.spanID && timingsLoading}
        ><span class="sr-only"
          >{planned ? 'Not started' : missingTiming(reference)}</span
        ></span
      >
    {/if}
    {#if span && origin !== undefined && maximumDuration > 1n}
      {@const node = traceIndex.nodes.get(referenceKey(span))}
      {#if node}
        <EvalTimelineMarkers
          records={evalTimelineRecords(
            node,
            timelineLogs.get(span.traceID.toLowerCase()) ?? []
          )}
          start={origin}
          end={origin + maximumDuration}
          spanStart={span.startTime}
          color={spanColors[referenceKey(span)] ??
            spanTimingColor(span, themeSignal.value)}
          onSelect={record => selectActivity(record, span)}
        />
      {/if}
    {/if}
  </TimingCell>
{/snippet}

<PaneHeader
  pill
  mode="title"
  title={experiment.name}
  ariaLabel="Evaluation results"
>
  {#snippet right()}<ExperimentStatusBadge
      state={experiment.state}
      pauseReason={experiment.pauseReason}
    />{/snippet}
  {#snippet badge()}
    <span
      class="text-sm text-subtle tabular-nums"
      title="Shown results / total results"
      >{results.length}/{experiment.results.length} results</span
    >
  {/snippet}
  {#snippet metaRight()}
    {@render headerActions?.()}
    <div
      class="header-choice-controls"
      role="radiogroup"
      aria-label="Eval view"
    >
      {#each views as item, index (item.id)}
        {@const label = item.id === 'cost' ? 'Cost · CAD est.' : item.label}
        <button
          type="button"
          role="radio"
          aria-label={label}
          class="btn btn-xs header-tab header-toggle tooltip tooltip-left"
          aria-checked={viewKey === item.id}
          disabled={!availableViews[item.id]}
          tabindex={viewKey === item.id ? 0 : -1}
          aria-controls={panelID}
          data-tip={availableViews[item.id]
            ? label
            : `${label}: no recorded data`}
          onclick={() => (viewKey = item.id)}
          onkeydown={event => selectView(event, index)}
        >
          <span class="header-tab__icon shrink-0">
            <HugeiconsIcon
              icon={item.icon}
              size={16}
              strokeWidth={1.5}
              aria-hidden="true"
            />
          </span>
          <span class="header-tab__label"
            >{item.label}{#if item.id === 'cost'}<span
                class:text-subtle={availableViews[item.id]}
                >{' · CAD est.'}</span
              >{/if}</span
          >
        </button>
      {/each}
    </div>
  {/snippet}
  {#snippet metaLeft()}
    {@render loadingStatus?.()}
    <LoadingStatus
      pending={timingsLoading || activityLoading}
      requestKey={timingsLoading ? timingRequest : activityRequest}
      label={timingsLoading ? 'Loading telemetry…' : 'Loading tool evidence…'}
      class="text-sm text-subtle"
    />
    {#if !activityLoading && activityFailed}
      <span class="truncate text-sm text-subtle" role="status"
        >Tool evidence could not be loaded.</span
      >
    {/if}
  {/snippet}
</PaneHeader>

{@render beforeTable?.()}

<div
  id={panelID}
  class="flex min-h-0 flex-1 flex-col"
  style:--tree-count-width={`max(var(--waterfall-gutter-col), calc(${countDigits + 2}ch + 0.5rem))`}
>
  <ResizableTable
    columns={columns.map(column => ({
      id: column.key,
      label: column.label,
      min: column.min,
      width: column.width,
      fixed: column.fixed,
    }))}
    storageKey="evals-column-widths"
    caption={`${viewLabel} for ${experiment.name}`}
    pinnedCount={columns.length > 2 ? 2 : 0}
    fillColumn="seconds"
    stableColumns={['repetition']}
    constrainToViewport
  >
    {#snippet header(layout)}
      <tr class="eval-results__head">
        {#each columns as column (column.key)}
          {#if column.key === 'name'}
            <th
              class="table-cell eval-results__identity-cell"
              scope="colgroup"
              colspan="2"
              aria-label="Experiment · Test case · Result"
              class:table-cell--pinned={layout.pinned.name !== undefined}
              style:left={layout.pinned.name === undefined
                ? undefined
                : `${layout.pinned.name}px`}
              aria-sort={activeSort?.key === 'name' ||
              activeSort?.key === 'started'
                ? activeSort.direction === 'asc'
                  ? 'ascending'
                  : 'descending'
                : 'none'}
            >
              <span class="eval-results__identity-heading">
                <span
                  class="eval-results__case-heading"
                  bind:offsetWidth={caseGutterWidth}
                >
                  <TableColumnHeading label="Experiment" />
                  <span aria-hidden="true">·</span>
                  <TableSortButton
                    label="Test case"
                    inset={false}
                    direction={activeSort?.key === 'name'
                      ? activeSort.direction
                      : undefined}
                    onclick={() => toggleSort('name')}
                    ><TableColumnHeading label="Test case" /></TableSortButton
                  >
                  <span aria-hidden="true">·</span>
                </span>
                <span
                  class="eval-results__result-heading"
                  bind:offsetWidth={resultGutterWidth}
                >
                  <TableSortButton
                    label="Started"
                    title="Sort results by available start time"
                    inset={false}
                    direction={activeSort?.key === 'started'
                      ? activeSort.direction
                      : undefined}
                    onclick={() => toggleSort('started')}
                    ><TableColumnHeading label="Result" /></TableSortButton
                  >
                </span>
              </span>
            </th>
          {:else if column.key === 'seconds' && maximumDuration <= 1n}
            <td class="table-cell eval-results__timing-cell" aria-hidden="true"
            ></td>
          {:else if column.key !== 'repetition'}
            <th
              class="table-cell"
              class:eval-results__timing-cell={column.key === 'seconds'}
              aria-label={column.key === 'seconds' ? 'Duration' : undefined}
              class:table-cell--pinned={layout.pinned[column.key] !== undefined}
              style:left={layout.pinned[column.key] === undefined
                ? undefined
                : `${layout.pinned[column.key]}px`}
              scope="col"
              aria-sort={activeSort?.key === column.key
                ? activeSort.direction === 'asc'
                  ? 'ascending'
                  : 'descending'
                : 'none'}
            >
              <TableSortButton
                label={column.label}
                inset={column.key !== 'seconds'}
                direction={activeSort?.key === column.key
                  ? activeSort.direction
                  : undefined}
                align={column.numeric ? 'right' : 'left'}
                title={column.hint}
                onclick={() => toggleSort(column.key)}
              >
                {#if column.key === 'seconds' && maximumDuration > 1n}
                  <TimingCell maximum={maximumDuration} header />
                {:else}<TableColumnHeading
                    label={column.label}
                    description={column.description}
                    align={column.numeric ? 'right' : 'left'}
                  />
                {/if}
              </TableSortButton>
            </th>
          {/if}
        {/each}
      </tr>
    {/snippet}
    {#snippet children(layout)}
      {#each visibleRows as row (row.id)}
        {@const item = row.result}
        {@const open = outlineOpen(row, enabledViews, overrides)}
        {@const selected = selectedRowID
          ? selectedRowID === row.id
          : selectedID
            ? row.kind === 'result' && selectedID === item?.id
            : row.kind === 'experiment'}
        <!-- Native row-name and duration buttons retain keyboard access. Tree controls stay independent. -->
        <!-- svelte-ignore a11y_click_events_have_key_events -->
        <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
        <tr
          class="table-row"
          onclick={event => selectRow(event, row)}
          class:eval-results__case={row.kind === 'case' ||
            row.kind === 'experiment'}
          class:table-row--selected={selected}
          data-outline-id={row.id}
          data-row-kind={row.kind}
          data-depth={row.depth}
          data-case-id={row.caseID}
          data-result-id={row.kind === 'result' ? item?.id : undefined}
          data-tool-result-id={row.kind === 'tool' ? item?.id : undefined}
          data-execution-id={row.kind === 'tool' || row.kind === 'grader'
            ? row.id
            : undefined}
          data-span-key={row.reference
            ? referenceKey(row.reference)
            : undefined}
        >
          <th
            colspan="2"
            scope="row"
            class="table-cell eval-results__case-cell text-left font-normal"
            class:table-cell--pinned={layout.pinned.name !== undefined}
            style:left={layout.pinned.name === undefined
              ? undefined
              : `${layout.pinned.name}px`}
          >
            <TreeRowIdentity
              name={row.name}
              depth={row.depth}
              tree={row.tree}
              subtreeCollapsed={!open}
              toggleLabel={`${open ? 'Collapse' : 'Expand'} ${row.name}${item && row.kind !== 'case' ? ` for ${item.name}` : ''}, ${row.tree.childrenCount} ${row.kind === 'experiment' ? 'test cases' : 'children'}`}
              onToggle={() => toggleOutline(row)}
            >
              {#snippet node()}{@render executionNode(
                  row.reference,
                  row.state,
                  row.internalFault
                )}{/snippet}
              <button
                type="button"
                onclick={event => selectRow(event, row)}
                aria-label={row.kind === 'result' && item?.sourceID
                  ? `${row.name} ${item.sourceID}`
                  : row.name}
                aria-pressed={selected}
                class="block w-full min-w-0 truncate text-left"
                title={row.state === 'Unknown'
                  ? row.name
                  : `${row.name} · ${completionLabel(row.state, row.internalFault)}`}
                >{row.name}{#if row.kind === 'result' && item?.sourceID}<span
                    class="ml-2 text-subtle"
                    ><Identifier value={item.sourceID} /></span
                  >{/if}</button
              >
            </TreeRowIdentity>
          </th>
          {#if row.kind === 'experiment'}
            {#each columns.slice(2) as column (column.key)}
              <td
                class="table-cell text-left tabular-nums"
                class:eval-results__timing-cell={column.key === 'seconds'}
              >
                {#if column.key === 'seconds'}
                  {@render spanBar(
                    row.reference,
                    experimentWindow?.startTime,
                    row.seconds,
                    row.spanError,
                    row.window,
                    row.name,
                    experiment.state === 'Planned',
                    row
                  )}
                {:else if column.key === 'cost'}
                  {@render rowCost(row.estimate)}
                {:else if column.key === 'investigatorCost' || column.key === 'graderCost'}
                  {@render rowCost(experimentCosts[column.key])}
                {:else if column.key === 'tools'}
                  {@render ownedToolCount(row)}
                {/if}
              </td>
            {/each}
          {:else if row.kind === 'case' && !item}
            {@render totals(
              groups.find(group => group.id === row.caseID)!,
              row
            )}
          {:else}
            {#each columns.slice(2) as column (column.key)}
              <td
                class="table-cell text-left tabular-nums"
                class:eval-results__timing-cell={column.key === 'seconds'}
              >
                {#if column.key === 'seconds'}
                  {@render spanBar(
                    row.reference,
                    experimentWindow?.startTime ??
                      (item ? resultWindows[item.id]?.startTime : undefined),
                    row.seconds,
                    row.spanError,
                    row.window,
                    row.name,
                    row.state === 'Planned',
                    row
                  )}
                {:else if column.assessment && item && (row.kind === 'case' || row.kind === 'result' || row.kind === 'grading' || row.assessmentID === column.assessment.id)}
                  {@const assessment = item.assessments.find(
                    score => score.id === column.assessment?.id
                  )}
                  <div class="eval-results__assessment">
                    <AssessmentBadge
                      {assessment}
                      maximum={column.assessment.maximum}
                    />
                    {#if assessment?.value != null}<span
                        class="shrink-0 text-sm text-subtle tabular-nums"
                        >({assessment.value}{column.assessment.maximum ===
                        undefined
                          ? ''
                          : `/${column.assessment.maximum}`})</span
                      >{/if}
                  </div>
                {:else if column.key === 'tools'}
                  {@render ownedToolCount(row)}
                {:else if column.key === 'cost'}
                  {@render rowCost(row.estimate)}
                {:else if item && (column.key === 'investigatorCost' || column.key === 'graderCost')}
                  {#if row.kind === 'result' || row.kind === 'case'}
                    {@render rowCost({
                      total: item[column.key] ?? null,
                      incomplete: item[column.key] === undefined,
                    })}
                  {:else if (column.key === 'investigatorCost' && (row.kind === 'execution' || row.kind === 'tool')) || (column.key === 'graderCost' && (row.kind === 'grading' || row.kind === 'grader'))}
                    {@render rowCost(row.estimate)}
                  {/if}
                {/if}
              </td>
            {/each}
          {/if}
        </tr>
      {/each}
      {#snippet totals(group: (typeof groups)[number], row: EvalOutlineRow)}
        {#each columns as column (column.key)}
          {#if column.assessment}
            <td class="table-cell"></td>
          {:else if column.key === 'seconds'}
            <td
              class="table-cell eval-results__timing-cell text-left tabular-nums"
              title={row.window
                ? 'Elapsed time across available result references'
                : group.allResults.length === 1
                  ? 'Result elapsed time'
                  : undefined}
            >
              {#if row.window || group.allResults.length === 1}
                {@render spanBar(
                  undefined,
                  experimentWindow?.startTime ?? row.window?.startTime,
                  row.seconds,
                  row.spanError,
                  row.window,
                  row.name,
                  row.state === 'Planned',
                  row
                )}
              {:else}<TimingCell maximum={maximumDuration} />{/if}
            </td>
          {:else if column.key === 'tools'}
            <td class="table-cell text-left tabular-nums">
              {@render ownedToolCount(row)}
            </td>
          {:else if column.key === 'cost' || column.key === 'investigatorCost' || column.key === 'graderCost'}
            <td class="table-cell text-left tabular-nums">
              {@render rowCost(
                column.key === 'cost' ? row.estimate : group.summary[column.key]
              )}
            </td>
          {/if}
        {/each}
      {/snippet}
    {/snippet}
    {#snippet footer(layout)}
      {#if results.length && viewKey === 'cost'}
        <tr aria-label="Estimated cost for shown results">
          {#each columns as column (column.key)}
            {#if column.key === 'name'}
              <th
                scope="row"
                class="table-cell text-left text-sm font-normal"
                class:table-cell--pinned={layout.pinned[column.key] !==
                  undefined}
                style:left={layout.pinned[column.key] === undefined
                  ? undefined
                  : `${layout.pinned[column.key]}px`}
                title={`${results.length} shown results`}>Total</th
              >
            {:else}
              <td
                class="table-cell text-left text-sm font-normal tabular-nums"
                class:table-cell--pinned={layout.pinned[column.key] !==
                  undefined}
                style:left={layout.pinned[column.key] === undefined
                  ? undefined
                  : `${layout.pinned[column.key]}px`}
              >
                {#if column.key === 'cost'}
                  {@render rowCost(combineEstimates(estimates))}
                {:else if column.key === 'investigatorCost' || column.key === 'graderCost'}
                  {@render rowCost(
                    estimates.find(item => item.key === column.key)!
                  )}
                {/if}
              </td>
            {/if}
          {/each}
        </tr>
      {/if}
    {/snippet}
  </ResizableTable>
</div>

<style lang="postcss">
  @reference "../../app.css";
  .table-row {
    cursor: pointer;
  }
  .eval-results__timing-cell {
    padding-block: 0;
    height: 1px;
  }
  .eval-results__span-bar {
    width: 100%;
    text-align: left;
  }
  .eval-results__identity-cell {
    padding-inline: 0;
    font-weight: 400;
    color: var(--color-subtle);
  }
  .eval-results__identity-heading {
    @apply inline-flex w-max items-center gap-1 whitespace-nowrap;
  }
  .eval-results__identity-heading :global(.table-sort) {
    @apply inline-flex w-auto flex-none items-center gap-1;
  }
  .eval-results__identity-heading :global(.table-sort__direction) {
    position: static;
    transform: none;
  }
  .eval-results__case-heading {
    @apply inline-flex flex-none items-center gap-1 pl-4;
  }
  .eval-results__result-heading {
    @apply inline-flex flex-none items-center;
  }
  .eval-results__assessment {
    @apply flex items-center gap-1 whitespace-nowrap;
  }
  .eval-results__case-cell {
    padding: 0 0.5rem;
    height: 1px;
  }
  .eval-results__case-cell :global(.tree-row-identity) {
    height: 100%;
  }
</style>

<script lang="ts">
  import type { ComponentProps } from 'svelte'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import {
    CheckListIcon,
    ToolsIcon,
    Coins01Icon,
  } from '@hugeicons/core-free-icons'
  import PaneHeader from '@/components/shared/PaneHeader.svelte'
  import AssessmentBadge from './AssessmentBadge.svelte'
  import RunStatusBadge from './RunStatusBadge.svelte'
  import EvalStatus from './EvalStatus.svelte'
  import { caseSummary } from './case-summary'
  import TableSortButton from '@/components/shared/TableSortButton.svelte'
  import TableColumnHeading from '@/components/shared/TableColumnHeading.svelte'
  import ResizableTable from '@/components/shared/ResizableTable.svelte'
  import TreeRowIdentity from '@/components/shared/TreeRowIdentity.svelte'
  import LegendRows from '@/components/shared/LegendRows.svelte'
  import DurationBar from '@/components/shared/DurationBar.svelte'
  import TimingCell from '@/components/shared/TimingCell.svelte'
  import { telemetryAPI } from '@/services/telemetry-service'
  import type { SpanTiming, TraceData } from '@/types/api-types'
  import { getServiceName } from '@/utils/resource'
  import {
    evalExecutionRows,
    evalTraceReferences,
    indexEvalTraces,
    referenceKey,
    visibleExecutionRows,
  } from './eval-traces'
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
    groupAttempts,
  } from './group-attempts'
  import {
    cost,
    estimatedCost,
    percent,
    type TelemetryReference,
    type AssessmentDefinition,
    type EvalAttempt,
    type EvalRun,
  } from './model'
  import { type AttemptSort, type AttemptSortKey } from './sort-attempts'

  let {
    run,
    attempts,
    selectedID,
  }: {
    run: EvalRun
    attempts: EvalAttempt[]
    selectedID?: string
  } = $props()
  const panelID = $props.id()
  const views = [
    { id: 'results', label: 'Results', icon: CheckListIcon },
    { id: 'time', label: 'Tool calls', icon: ToolsIcon },
    { id: 'cost', label: 'Cost', icon: Coins01Icon },
  ]
  let enabledViews = $state(['results'])
  const shownViews = $derived(
    views.filter(item => enabledViews.includes(item.id))
  )
  const viewKey = $derived(shownViews.map(item => item.id).join('-') || 'case')
  const viewLabel = $derived(
    shownViews.map(item => item.label).join(' + ') || 'Cases'
  )
  function toggleView(id: string) {
    enabledViews = enabledViews.includes(id)
      ? enabledViews.filter(value => value !== id)
      : [...enabledViews, id]
  }
  type LegendEntry = Pick<ComponentProps<typeof EvalStatus>, 'label' | 'tone'>
  const completionLegend: LegendEntry[] = [
    { label: 'Complete', tone: 'complete' },
    { label: 'In progress', tone: 'running' },
    { label: 'Span error', tone: 'error' },
    { label: 'Internal fault', tone: 'papaya' },
    { label: 'Planned', tone: 'neutral' },
  ]
  let caseGutterWidth = $state(0)
  let repGutterWidth = $state(0)
  const countDigits = $derived.by(() => {
    const repetitions = new Map<string, number>()
    let maximum = 1
    for (const attempt of run.attempts) {
      const count = (repetitions.get(attempt.caseID) ?? 0) + 1
      repetitions.set(attempt.caseID, count)
      maximum = Math.max(
        maximum,
        count,
        attempt.tools.length +
          attempt.assessments.filter(item => item.telemetry?.spanID).length
      )
    }
    return String(maximum).length
  })
  let collapsedCases = $state<string[]>([])
  let expandedAttempts = $state<string[]>([])
  let traceViews = $state<TraceData[]>([])
  const traceIndex = $derived(indexEvalTraces(traceViews))
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
      run.attempts.map(attempt => [
        attempt.id,
        evalExecutionRows(attempt, traceIndex),
      ])
    )
  )
  let collapsedSpans = $state<string[]>([])
  let timingsLoading = $state(false)
  let timingsFailed = $state(false)
  const timingKey = referenceKey
  const timingFor = (reference?: TelemetryReference) =>
    reference?.spanID ? spanTimings[timingKey(reference)] : undefined
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
    if (timingsLoading) return 'Loading span'
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
  const attemptWindows = $derived(
    Object.fromEntries(
      run.attempts.map(attempt => [
        attempt.id,
        spanWindow([
          timingFor(attempt.telemetry),
          ...attempt.tools.map(tool => timingFor(tool.telemetry)),
          ...attempt.assessments.map(assessment =>
            timingFor(assessment.telemetry)
          ),
        ]),
      ])
    )
  )
  const runWindow = $derived(
    timingFor(run.telemetry)
      ? spanWindow(Object.values(spanTimings))
      : undefined
  )
  const maximumDuration = $derived.by(() => {
    const extent = runWindow
      ? runWindow.endTime - runWindow.startTime
      : Object.values(attemptWindows).reduce((maximum, window) => {
          const extent = window ? window.endTime - window.startTime : 0n
          return extent > maximum ? extent : maximum
        }, 0n)
    return [
      run,
      ...run.attempts.flatMap(attempt => [attempt, ...attempt.tools]),
    ].reduce((maximum, item) => {
      const duration = durationFor(item.telemetry, item.seconds) ?? 0n
      return duration > maximum ? duration : maximum
    }, extent)
  })
  $effect(() => {
    const references = evalTraceReferences(run)
    const controller = new AbortController()
    traceViews = []
    timingsLoading = true
    timingsFailed = false
    void telemetryAPI
      .getReferencedTraceViews(references, controller.signal)
      .then(
        result => {
          if (controller.signal.aborted) return
          traceViews = result.traces
          timingsFailed = result.unavailableTraceIDs.length > 0
          timingsLoading = false
        },
        () => {
          if (controller.signal.aborted) return
          timingsLoading = false
          timingsFailed = true
        }
      )
    return () => controller.abort()
  })
  function toggleCase(id: string) {
    collapsedCases = collapsedCases.includes(id)
      ? collapsedCases.filter(key => key !== id)
      : [...collapsedCases, id]
  }
  type Column = {
    key: AttemptSortKey
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
  const options = $derived<Column[]>([
    {
      key: 'name',
      label: 'Case',
      group: 'Case',
      width: Math.max(1, caseGutterWidth),
      min: Math.max(1, caseGutterWidth),
      fixed: true,
    },
    {
      key: 'repetition',
      label: 'Rep',
      hint: 'Repetition',
      group: 'Case',
      width: Math.max(192, repGutterWidth),
      min: Math.max(144, repGutterWidth),
      numeric: true,
    },
    {
      key: 'seconds',
      label: 'Duration',
      hint: runWindow
        ? 'Recorded starts and durations on one timeline across this run'
        : 'Recorded starts within each question, with one duration scale across questions',
      group: 'Duration',
      width: 288,
      min: 224,
    },
    {
      key: 'tools',
      label: 'Tool calls',
      group: 'Tool calls',
      width: 112,
      min: 104,
    },
    {
      key: 'failures',
      label: 'Failures',
      hint: 'Failed tool calls',
      group: 'Tool calls',
      width: 104,
      min: 96,
    },
    ...run.assessments.map(assessment => ({
      key: `assessment:${assessment.id}` as AttemptSortKey,
      label: assessment.name,
      group: 'Results',
      width: 192,
      min: 160,
      hint: assessment.scale,
      assessment,
    })),
    {
      key: 'investigatorCost',
      label: 'Investigator',
      group: 'Cost',
      width: 160,
      min: 144,
      description: 'Estimate · CAD',
    },
    {
      key: 'graderCost',
      label: 'Grader',
      group: 'Cost',
      width: 160,
      min: 144,
      description: 'Estimate · CAD',
    },
  ])

  const columns = $derived(
    options.filter(
      column =>
        column.group === 'Case' ||
        column.key === 'seconds' ||
        shownViews.some(view => view.label === column.group)
    )
  )
  let sorts = $state<Record<string, AttemptSort | undefined>>({})
  const activeSort = $derived(sorts[viewKey])
  const measuredAttempts = $derived(
    run.attempts.map(attempt => {
      const duration = durationFor(attempt.telemetry, attempt.seconds)
      return {
        ...attempt,
        state: execution[attempt.id]?.state ?? attempt.state,
        seconds: duration !== undefined ? Number(duration) / 1e9 : undefined,
      }
    })
  )
  const groups = $derived(
    groupAttempts(
      measuredAttempts.filter(item =>
        attempts.some(attempt => attempt.id === item.id)
      ),
      measuredAttempts,
      activeSort
    ).map(group => ({
      ...group,
      internalFault: group.allAttempts.some(
        item => execution[item.id]?.internalFault
      ),
      summary: caseSummary(group.allAttempts, run.assessments),
      duration: group.allAttempts.reduce((total, attempt) => {
        return total + (durationFor(attempt.telemetry, attempt.seconds) ?? 0n)
      }, 0n),
    }))
  )

  function toggleSort(key: AttemptSortKey) {
    sorts[viewKey] = {
      key,
      direction:
        activeSort?.key === key && activeSort.direction === 'asc'
          ? 'desc'
          : 'asc',
    }
  }
  const estimates = $derived([
    { key: 'investigatorCost', ...estimatedCost(attempts, 'investigatorCost') },
    { key: 'graderCost', ...estimatedCost(attempts, 'graderCost') },
  ])
</script>

{#snippet executionNode(
  reference: TelemetryReference | undefined,
  state: EvalAttempt['state'],
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
    <EvalStatus label={state} dotOnly tone={completionTone(state)} />
  {/if}
{/snippet}

{#snippet spanBar(
  reference?: TelemetryReference,
  origin?: bigint,
  seconds?: number,
  spanError = false
)}
  {@const span = timingFor(reference)}
  {@const duration = durationFor(reference, seconds)}
  <TimingCell maximum={maximumDuration}>
    {#if span && span.endTime >= span.startTime}<span
        class="eval-results__span-bar block"
        role="img"
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
        /></span
      >
    {:else if duration !== undefined}
      <span
        class="block"
        title="Recorded eval duration"
        style:--bar-color={spanError ? 'var(--color-error)' : 'currentColor'}
      >
        <DurationBar {duration} maximum={maximumDuration} />
      </span>
    {:else}
      <span
        class="block min-h-[var(--signal-card-row-h)]"
        title={missingTiming(reference)}
        aria-label={missingTiming(reference)}
        aria-busy={!!reference?.spanID && timingsLoading}
      ></span>
    {/if}
  </TimingCell>
{/snippet}

<PaneHeader pill mode="title" title={run.name}>
  {#snippet right()}<RunStatusBadge
      state={run.state}
      pauseReason={run.pauseReason}
    />{/snippet}
  {#snippet badge()}
    <span
      class="text-sm text-rp-subtle tabular-nums"
      title="Shown attempts / total attempts"
      >{attempts.length}/{run.attempts.length} attempts</span
    >
  {/snippet}
  {#snippet metaRight()}
    <div class="header-controls" role="group" aria-label="Shown eval columns">
      {#each views as item (item.id)}
        <button
          type="button"
          class="btn btn-xs header-tab header-toggle tooltip tooltip-left"
          aria-pressed={enabledViews.includes(item.id)}
          aria-controls={panelID}
          data-tip={item.label}
          onclick={() => toggleView(item.id)}
        >
          <span class="header-tab__icon shrink-0">
            <HugeiconsIcon
              icon={item.icon}
              size={16}
              strokeWidth={1.5}
              aria-hidden="true"
            />
          </span>
          <span class="header-tab__label">{item.label}</span>
        </button>
      {/each}
    </div>
  {/snippet}
  {#snippet metaLeft()}
    {#if shownViews.length}
      <LegendRows
        items={completionLegend}
        label="Execution status legend"
        itemKey={entry => entry.label}
      >
        {#snippet children(entry)}<EvalStatus
            label={entry.label}
            tone={entry.tone}
          />{/snippet}
      </LegendRows>
    {/if}
  {/snippet}
</PaneHeader>

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
    caption={`${viewLabel} for ${run.name}`}
    pinnedCount={columns.length > 2 ? 2 : 0}
    fillColumn="seconds"
    stableColumns={['repetition']}
    constrainToViewport
  >
    {#snippet header(layout)}
      <tr>
        {#each columns as column (column.key)}
          {#if column.key === 'name'}
            <th
              class="table-cell eval-results__identity-cell"
              scope="colgroup"
              colspan="2"
              aria-label="Case · Rep"
              class:table-cell--pinned={layout.pinned.name !== undefined}
              style:left={layout.pinned.name === undefined
                ? undefined
                : `${layout.pinned.name}px`}
              aria-sort={activeSort?.key === 'name' ||
              activeSort?.key === 'repetition'
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
                  <TableSortButton
                    label="Case"
                    direction={activeSort?.key === 'name'
                      ? activeSort.direction
                      : undefined}
                    onclick={() => toggleSort('name')}
                    ><TableColumnHeading label="Case" /></TableSortButton
                  >
                  <span aria-hidden="true">·</span>
                </span>
                <span
                  class="eval-results__rep-heading"
                  bind:offsetWidth={repGutterWidth}
                >
                  <TableSortButton
                    label="Repetition"
                    title="Repetition"
                    align="right"
                    direction={activeSort?.key === 'repetition'
                      ? activeSort.direction
                      : undefined}
                    onclick={() => toggleSort('repetition')}
                    ><TableColumnHeading label="Rep" /></TableSortButton
                  >
                </span>
              </span>
            </th>
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
                {#if column.key === 'seconds'}
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
      {#if run.telemetry || run.seconds !== undefined}
        <tr class="table-row" aria-label="Run span">
          <th
            colspan="2"
            scope="row"
            class="table-cell text-left font-normal"
            class:table-cell--pinned={layout.pinned.name !== undefined}
            style:left={layout.pinned.name === undefined
              ? undefined
              : `${layout.pinned.name}px`}>{run.name}</th
          >
          {#each columns.slice(2) as column (column.key)}
            <td
              class="table-cell"
              class:eval-results__timing-cell={column.key === 'seconds'}
            >
              {#if column.key === 'seconds'}
                {@render spanBar(
                  run.telemetry,
                  runWindow?.startTime,
                  run.seconds,
                  Object.values(execution).some(item => item.spanError)
                )}
              {/if}
            </td>
          {/each}
        </tr>
      {/if}
      {#each groups as group (group.id)}
        {@const single = group.allAttempts.length === 1}
        {#if !single}
          <tr class="table-row eval-results__case" data-case-id={group.id}>
            <th
              scope="row"
              colspan="2"
              class="table-cell eval-results__case-cell text-left font-normal"
              class:table-cell--pinned={layout.pinned.name !== undefined}
              style:left={layout.pinned.name === undefined
                ? undefined
                : `${layout.pinned.name}px`}
            >
              <TreeRowIdentity
                name={group.name}
                depth={0}
                tree={{
                  childrenCount: group.attempts.length,
                  ancestorHasNextSibling: [],
                  isLastChild: true,
                }}
                subtreeCollapsed={collapsedCases.includes(group.id)}
                toggleLabel={`${collapsedCases.includes(group.id) ? 'Expand' : 'Collapse'} ${group.name}, ${group.attempts.length} ${group.attempts.length === 1 ? 'repetition' : 'repetitions'}`}
                onToggle={() => toggleCase(group.id)}
              >
                {#snippet node()}<EvalStatus
                    label={completionLabel(group.state, group.internalFault)}
                    dotOnly
                    tone={completionTone(group.state, group.internalFault)}
                  />{/snippet}
                <span class="block truncate">{group.name}</span>
              </TreeRowIdentity>
            </th>
            {@render totals(group)}
          </tr>
        {/if}
        {#if single || !collapsedCases.includes(group.id)}
          {#each group.attempts as item, index (item.id)}
            <tr
              class="table-row eval-results__attempt"
              class:eval-results__single={single}
              data-attempt-id={item.id}
              data-case-id={group.id}
              class:table-row--selected={selectedID === item.id}
            >
              {#each columns as column (column.key)}
                {#if column.key === 'name'}
                  <th
                    colspan="2"
                    scope="row"
                    class="table-cell eval-results__case-cell font-normal"
                    class:table-cell--pinned={layout.pinned.name !== undefined}
                    style:left={layout.pinned.name === undefined
                      ? undefined
                      : `${layout.pinned.name}px`}
                  >
                    <TreeRowIdentity
                      name={item.name}
                      depth={single ? 0 : 1}
                      tree={{
                        childrenCount: execution[item.id]?.childrenCount ?? 0,
                        ancestorHasNextSibling: single ? [] : [true],
                        isLastChild: index === group.attempts.length - 1,
                      }}
                      subtreeCollapsed={!expandedAttempts.includes(item.id)}
                      toggleLabel={`${expandedAttempts.includes(item.id) ? 'Collapse' : 'Expand'} execution for ${item.name}, repetition ${item.repetition}, ${execution[item.id]?.rows.length ?? 0} references`}
                      onToggle={() =>
                        (expandedAttempts = expandedAttempts.includes(item.id)
                          ? expandedAttempts.filter(id => id !== item.id)
                          : [...expandedAttempts, item.id])}
                    >
                      {#snippet node()}{@render executionNode(
                          item.telemetry,
                          item.state,
                          execution[item.id]?.internalFault
                        )}{/snippet}
                      <span
                        class={single
                          ? 'block w-full min-w-0 truncate text-left text-base-content'
                          : 'eval-results__rep-label'}
                        title={`${item.name} — Rep ${item.repetition} · ${completionLabel(item.state, execution[item.id]?.internalFault)}`}
                        aria-describedby={`${panelID}-status-${item.id}`}
                        >{#if single}{item.name}{:else}<span class="sr-only"
                            >{item.name}</span
                          >Rep {item.repetition}{/if}</span
                      >
                      <span id={`${panelID}-status-${item.id}`} class="sr-only"
                        >Rep {item.repetition} · {completionLabel(
                          item.state,
                          execution[item.id]?.internalFault
                        )}</span
                      >
                    </TreeRowIdentity>
                  </th>
                {:else if column.key === 'repetition'}
                  <!-- Tool branches use one identity cell spanning Case and Rep. -->
                {:else if column.assessment}
                  {@const assessment = item.assessments.find(
                    score => score.id === column.assessment?.id
                  )}
                  <td class="table-cell text-left">
                    <div class="eval-results__assessment">
                      <AssessmentBadge
                        {assessment}
                        maximum={column.assessment.maximum}
                      />
                      {#if assessment?.value != null}<span
                          class="shrink-0 text-sm text-rp-subtle tabular-nums"
                          >({assessment.value}/{column.assessment
                            .maximum})</span
                        >{/if}
                    </div>
                  </td>
                {:else if column.key === 'seconds'}
                  <td
                    class="table-cell eval-results__timing-cell text-left tabular-nums"
                  >
                    {@render spanBar(
                      item.telemetry,
                      runWindow?.startTime ??
                        attemptWindows[item.id]?.startTime,
                      item.seconds,
                      execution[item.id]?.spanError
                    )}
                  </td>
                {:else if column.key === 'tools'}
                  <td class="table-cell text-left tabular-nums"
                    >{item.tools.length}</td
                  >
                {:else if column.key === 'failures'}
                  {@const failed = item.tools.filter(
                    tool => tool.state === 'Error'
                  ).length}
                  <td class="table-cell text-left tabular-nums">{failed}</td>
                {:else if column.key === 'investigatorCost' || column.key === 'graderCost'}
                  <td class="table-cell text-left tabular-nums"
                    >{cost(item[column.key])}</td
                  >
                {/if}
              {/each}
            </tr>
            {#if expandedAttempts.includes(item.id)}
              {#each visibleExecutionRows(execution[item.id]?.rows ?? [], collapsedSpans) as tool (tool.id)}
                <tr
                  class="table-row eval-results__attempt"
                  data-tool-attempt-id={item.id}
                  data-execution-id={tool.id}
                  data-span-key={tool.reference
                    ? referenceKey(tool.reference)
                    : undefined}
                >
                  <th
                    colspan="2"
                    scope="row"
                    class="table-cell eval-results__case-cell font-normal"
                    class:table-cell--pinned={layout.pinned.name !== undefined}
                    style:left={layout.pinned.name === undefined
                      ? undefined
                      : `${layout.pinned.name}px`}
                  >
                    <TreeRowIdentity
                      name={tool.name}
                      depth={tool.depth + (single ? 0 : 1)}
                      tree={{
                        ...tool.tree,
                        ancestorHasNextSibling: single
                          ? tool.tree.ancestorHasNextSibling
                          : [
                              index < group.attempts.length - 1,
                              ...tool.tree.ancestorHasNextSibling,
                            ],
                      }}
                      subtreeCollapsed={collapsedSpans.includes(tool.id)}
                      toggleLabel={`${collapsedSpans.includes(tool.id) ? 'Expand' : 'Collapse'} ${tool.name}, ${tool.tree.childrenCount} referenced spans`}
                      onToggle={() =>
                        (collapsedSpans = collapsedSpans.includes(tool.id)
                          ? collapsedSpans.filter(id => id !== tool.id)
                          : [...collapsedSpans, tool.id])}
                    >
                      {#snippet node()}{@render executionNode(
                          tool.reference,
                          tool.state
                        )}{/snippet}
                      <span class="block min-w-0 truncate">{tool.name}</span>
                    </TreeRowIdentity>
                  </th>
                  {#each columns.slice(2) as column (column.key)}
                    <td
                      class="table-cell"
                      class:eval-results__timing-cell={column.key === 'seconds'}
                    >
                      {#if column.key === 'seconds'}
                        {@render spanBar(
                          tool.reference,
                          runWindow?.startTime ??
                            attemptWindows[item.id]?.startTime,
                          tool.seconds,
                          tool.state === 'Error'
                        )}
                      {/if}
                    </td>
                  {/each}
                </tr>
              {/each}
            {/if}
          {/each}
        {/if}
      {/each}
      {#snippet totals(group: (typeof groups)[number])}
        {#each columns as column (column.key)}
          {#if column.assessment}
            {@const score = group.summary.scores[column.assessment.id]}
            <td
              class="table-cell text-left tabular-nums"
              title={`${score.scored} scored / ${score.denominator} denominator · all case repetitions`}
            >
              <EvalStatus
                label={percent(score.percent)}
                tone={score.percent === null
                  ? 'neutral'
                  : score.percent === 100
                    ? 'success'
                    : score.percent === 0
                      ? 'error'
                      : 'warning'}
              />
            </td>
          {:else if column.key === 'seconds'}
            <td
              class="table-cell eval-results__timing-cell text-left tabular-nums"
              title="Sum of recorded question durations across all repetitions"
            >
              <TimingCell maximum={maximumDuration}>
                {group.summary.duration.total === null
                  ? ''
                  : formatDuration(group.duration)}
                {#if group.summary.duration.incomplete && group.summary.duration.total !== null}<span
                    class="block text-rp-subtle">Partial total</span
                  >{/if}
              </TimingCell>
            </td>
          {:else if column.key === 'tools'}
            <td
              class="table-cell text-left tabular-nums"
              title="Recorded tool calls across all repetitions"
            >
              {group.summary.calls}
            </td>
          {:else if column.key === 'failures'}
            <td
              class="table-cell text-left tabular-nums"
              title="Failed tool calls across all repetitions"
              >{group.summary.failures}</td
            >
          {:else if column.key === 'investigatorCost' || column.key === 'graderCost'}
            {@const estimate = group.summary[column.key]}
            <td class="table-cell text-left tabular-nums">
              {cost(estimate.total)}
              {#if estimate.incomplete && estimate.total !== null}<span
                  class="block text-rp-subtle">Partial estimate</span
                >{/if}
            </td>
          {/if}
        {/each}
      {/snippet}
    {/snippet}
    {#snippet footer(layout)}
      {#if attempts.length && columns.some( column => estimates.some(estimate => estimate.key === column.key) )}
        <tr aria-label="Estimated cost for shown attempts">
          {#each columns as column (column.key)}
            {@const estimate = estimates.find(
              estimate => estimate.key === column.key
            )}
            {#if column.key === 'name'}
              <th
                scope="row"
                class="table-cell text-left text-sm font-normal"
                class:table-cell--pinned={layout.pinned[column.key] !==
                  undefined}
                style:left={layout.pinned[column.key] === undefined
                  ? undefined
                  : `${layout.pinned[column.key]}px`}
                title={`${attempts.length} shown attempts`}>Total</th
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
                {#if estimate}
                  {cost(estimate.total)}
                  {#if estimate.incomplete && estimate.total !== null}<span
                      class="block text-rp-subtle">Partial estimate</span
                    >{/if}
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
  .eval-results__timing-cell {
    padding-block: 0;
    height: 1px;
  }
  .eval-results__identity-cell {
    padding-inline: 0;
    font-weight: 400;
    color: var(--color-subtle);
  }
  .eval-results__identity-heading {
    @apply inline-flex w-max items-center;
  }
  .eval-results__case-heading {
    @apply inline-flex items-center gap-1 pl-4 pr-1;
    min-width: calc(
      0.5rem + var(--tree-count-width) + var(--waterfall-gutter-col)
    );
  }
  .eval-results__rep-heading {
    @apply block pr-4;
    min-width: var(--waterfall-gutter-col);
  }
  .eval-results__assessment {
    @apply flex items-center gap-1 whitespace-nowrap;
  }
  .eval-results__attempt:not(.eval-results__single) :global(.table-cell),
  .eval-results__attempt:not(.eval-results__single) :global(.eval-status) {
    color: var(--color-subtle);
  }
  .eval-results__case-cell {
    padding: 0 0.5rem;
  }
  .eval-results__rep-label {
    @apply min-h-[var(--signal-card-row-h)] shrink-0 text-right text-sm font-normal;
    color: var(--color-subtle);
  }
</style>

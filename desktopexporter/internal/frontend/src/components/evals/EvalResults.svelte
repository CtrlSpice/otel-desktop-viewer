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
    groupAttempts,
  } from './group-attempts'
  import {
    estimatedCost,
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
    selectedRowID,
    onSelect,
    onSelectRun,
    onResolveSelection,
  }: {
    run: EvalRun
    attempts: EvalAttempt[]
    selectedID?: string
    selectedRowID?: string
    onSelect?: (row: EvalOutlineRow) => void
    onSelectRun?: () => void
    onResolveSelection?: (row: EvalOutlineRow | undefined) => void
  } = $props()
  const panelID = $props.id()
  const views = [
    { id: 'results', label: 'Results', icon: CheckListIcon },
    { id: 'time', label: 'Tool calls', icon: ToolsIcon },
    { id: 'cost', label: 'Cost', icon: Coins01Icon },
  ]
  let viewKey = $state('results')
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
    const next = (index + direction + views.length) % views.length
    viewKey = views[next].id
    const button = event.currentTarget as HTMLButtonElement
    button.parentElement
      ?.querySelectorAll<HTMLButtonElement>('[role="radio"]')
      [next]?.focus()
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
  let disclosure = $state<Record<string, Record<string, boolean>>>({})
  const overrides = $derived(disclosure[run.id] ?? {})
  function toggleOutline(row: EvalOutlineRow) {
    disclosure = {
      ...disclosure,
      [run.id]: {
        ...overrides,
        [row.id]: !outlineOpen(row, enabledViews, overrides),
      },
    }
  }
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
          timingFor(attempt.attemptTelemetry),
          ...attempt.tools.map(tool => timingFor(tool.telemetry)),
          ...attempt.assessments.map(assessment =>
            timingFor(assessment.telemetry)
          ),
        ]),
      ])
    )
  )
  const runWindow = $derived(spanWindow(Object.values(spanTimings)))
  const maximumDuration = $derived.by(() => {
    const extent = runWindow ? runWindow.endTime - runWindow.startTime : 0n
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
  const headline = $derived(
    run.assessments.find(item => item.id === run.headlineAssessment) ??
      run.assessments[0]
  )
  const columns = $derived<Column[]>([
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
      width: Math.max(320, repGutterWidth),
      min: Math.max(144, repGutterWidth),
      numeric: true,
    },
    {
      key: 'seconds',
      label: 'Duration',
      hint: runWindow
        ? 'Recorded starts and durations on one timeline across this run'
        : 'Recorded durations; start offsets unavailable',
      group: 'Duration',
      width: 288,
      min: 224,
    },
    ...(viewKey === 'time'
      ? [
          {
            key: 'tools',
            label: 'Tool calls',
            group: 'Tool calls',
            width: 176,
            min: 144,
          } as Column,
        ]
      : viewKey === 'results'
        ? [
            {
              key: headline ? `assessment:${headline.id}` : 'state',
              label: headline?.name ?? 'Result',
              group: 'Results',
              width: 160,
              min: 144,
              hint: headline?.scale,
              assessment: headline,
            } as Column,
          ]
        : [
            {
              key: 'investigatorCost',
              label: 'Investigator',
              group: 'Cost',
              width: 160,
              min: 144,
              description: 'Estimate · CAD',
            } as Column,
            {
              key: 'graderCost',
              label: 'Grader',
              group: 'Cost',
              width: 160,
              min: 144,
              description: 'Estimate · CAD',
            } as Column,
            {
              key: 'cost',
              label: 'Total cost',
              group: 'Cost',
              width: 160,
              min: 144,
              description: 'Estimate · CAD',
            } as Column,
          ]),
  ])
  let sorts = $state<Record<string, AttemptSort | undefined>>({})
  const activeSort = $derived(sorts[viewKey])
  const measuredAttempts = $derived(
    run.attempts.map(attempt => {
      const window = attemptWindows[attempt.id]
      const duration =
        durationFor(attempt.attemptTelemetry) ??
        (window
          ? window.endTime - window.startTime
          : durationFor(attempt.telemetry, attempt.seconds))
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
        return (
          total + (durationFor(attempt.attemptTelemetry, attempt.seconds) ?? 0n)
        )
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
  const outline = $derived(buildEvalOutline(groups, run, traceIndex))
  const visibleRows = $derived(
    visibleOutlineRows(outline, enabledViews, overrides)
  )
  function selectBar(event: MouseEvent) {
    const id = (event.currentTarget as HTMLElement).closest<HTMLElement>(
      '[data-outline-id]'
    )?.dataset.outlineId
    const row = outline.find(item => item.id === id)
    if (row) onSelect?.(row)
    else onSelectRun?.()
  }
  $effect(() => {
    onResolveSelection?.(outline.find(row => row.id === selectedRowID))
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
  spanError = false,
  window?: TimeWindow,
  label = 'Referenced execution window',
  planned = false
)}
  {@const span = timingFor(reference)}
  {@const duration = durationFor(reference, seconds)}
  <TimingCell
    maximum={maximumDuration}
    unavailable={!planned && !window && duration === undefined}
  >
    {#if window && (!span || span.endTime < span.startTime)}<button
        type="button"
        onclick={selectBar}
        class="eval-results__span-bar block"
        aria-label={`${label}, ${formatDuration(window.endTime - window.startTime)}`}
        title={`${label} · elapsed time across available references · Start offset ${formatDuration(window.startTime - (origin ?? window.startTime))}`}
        style:--bar-color={spanError
          ? 'var(--color-error)'
          : 'var(--color-secondary)'}
        ><DurationBar
          duration={window.endTime - window.startTime}
          maximum={maximumDuration}
          offset={window.startTime - (origin ?? window.startTime)}
        /></button
      >
    {:else if span && span.endTime >= span.startTime}<button
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
    {:else if duration !== undefined}
      <button
        type="button"
        onclick={selectBar}
        class="block w-full text-left"
        title="Recorded eval duration"
        style:--bar-color={spanError ? 'var(--color-error)' : 'currentColor'}
      >
        <DurationBar {duration} maximum={maximumDuration} />
      </button>
    {:else}
      <span
        class="block min-h-[var(--signal-card-row-h)]"
        title={planned ? 'Not started' : missingTiming(reference)}
        aria-label={planned ? 'Not started' : missingTiming(reference)}
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
    <div class="header-controls" role="radiogroup" aria-label="Eval view">
      {#each views as item, index (item.id)}
        <button
          type="button"
          role="radio"
          class="btn btn-xs header-tab header-toggle tooltip tooltip-left"
          aria-checked={viewKey === item.id}
          tabindex={viewKey === item.id ? 0 : -1}
          aria-controls={panelID}
          data-tip={item.label}
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
          <span class="header-tab__label">{item.label}</span>
        </button>
      {/each}
    </div>
  {/snippet}
  {#snippet metaLeft()}
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
                    inset={false}
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
                    inset={false}
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
              : `${layout.pinned.name}px`}
            ><span class="truncate">{run.name}</span></th
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
                  outline.some(item => item.spanError)
                )}
              {:else if column.key === 'cost'}
                {@render rowCost(combineEstimates(estimates))}
              {:else if column.key === 'investigatorCost' || column.key === 'graderCost'}
                {@render rowCost(
                  estimates.find(item => item.key === column.key)!
                )}
              {:else if column.key === 'tools'}
                {@render toolCount(
                  attempts.reduce((sum, item) => sum + item.tools.length, 0),
                  attempts.reduce(
                    (sum, item) =>
                      sum +
                      item.tools.filter(tool => tool.state === 'Error').length,
                    0
                  )
                )}
              {/if}
            </td>
          {/each}
        </tr>
      {/if}
      {#each visibleRows as row (row.id)}
        {@const group = groups.find(group => group.id === row.caseID)!}
        {@const item = row.attempt}
        {@const open = outlineOpen(row, enabledViews, overrides)}
        <tr
          class="table-row"
          class:eval-results__case={row.kind === 'case'}
          class:eval-results__attempt={row.kind !== 'case'}
          class:table-row--selected={selectedRowID
            ? selectedRowID === row.id
            : row.kind === 'attempt' && selectedID === item?.id}
          data-outline-id={row.id}
          data-row-kind={row.kind}
          data-depth={row.depth}
          data-case-id={row.caseID}
          data-attempt-id={row.kind === 'attempt' ? item?.id : undefined}
          data-tool-attempt-id={row.kind === 'tool' ? item?.id : undefined}
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
              toggleLabel={`${open ? 'Collapse' : 'Expand'} ${row.name}${item ? ` for ${item.name}, repetition ${item.repetition}` : ''}, ${row.tree.childrenCount} ${row.kind === 'case' ? 'attempts' : 'children'}`}
              onToggle={() => toggleOutline(row)}
            >
              {#snippet node()}{@render executionNode(
                  row.reference,
                  row.state,
                  row.internalFault
                )}{/snippet}
              <button
                type="button"
                onclick={() => onSelect?.(row)}
                class="block w-full min-w-0 truncate text-left"
                class:eval-results__rep-label={row.kind === 'attempt'}
                title={`${row.name} · ${completionLabel(row.state, row.internalFault)}`}
                >{row.name}</button
              >
            </TreeRowIdentity>
          </th>
          {#if row.kind === 'case'}
            {@render totals(group, row)}
          {:else}
            {#each columns.slice(2) as column (column.key)}
              <td
                class="table-cell text-left tabular-nums"
                class:eval-results__timing-cell={column.key === 'seconds'}
              >
                {#if column.key === 'seconds'}
                  {@render spanBar(
                    row.reference,
                    runWindow?.startTime ??
                      (item ? attemptWindows[item.id]?.startTime : undefined),
                    row.seconds,
                    row.spanError,
                    row.window,
                    row.name,
                    row.state === 'Planned'
                  )}
                {:else if column.assessment && item && (row.kind === 'attempt' || row.kind === 'grading' || row.assessmentID === column.assessment.id)}
                  {@const assessment = item.assessments.find(
                    score => score.id === column.assessment?.id
                  )}
                  <div class="eval-results__assessment">
                    <AssessmentBadge
                      {assessment}
                      maximum={column.assessment.maximum}
                    />
                    {#if assessment?.value != null}<span
                        class="shrink-0 text-sm text-rp-subtle tabular-nums"
                        >({assessment.value}/{column.assessment.maximum})</span
                      >{/if}
                  </div>
                {:else if column.key === 'tools' && item && (row.kind === 'attempt' || row.kind === 'execution')}
                  {@render toolCount(
                    item.tools.length,
                    item.tools.filter(tool => tool.state === 'Error').length
                  )}
                {:else if column.key === 'tools' && row.kind === 'tool'}
                  {@render toolCount(1, row.spanError ? 1 : 0)}
                {:else if column.key === 'cost'}
                  {@render rowCost(row.estimate)}
                {:else if item && (column.key === 'investigatorCost' || column.key === 'graderCost')}
                  {#if row.kind === 'attempt'}
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
              title={group.allAttempts.length === 1
                ? 'Attempt elapsed time'
                : 'Sum of attempt durations across all repetitions'}
            >
              {#if group.allAttempts.length === 1}
                {@render spanBar(
                  undefined,
                  runWindow?.startTime ?? row.window?.startTime,
                  row.seconds,
                  row.spanError,
                  row.window,
                  row.name,
                  row.state === 'Planned'
                )}
              {:else}<TimingCell
                  maximum={maximumDuration}
                  unavailable={row.state !== 'Planned' &&
                    group.summary.duration.total === null}
                >
                  {group.summary.duration.total === null
                    ? ''
                    : formatDuration(group.duration)}
                  {#if group.summary.duration.incomplete && group.summary.duration.total !== null}<span
                      class="block text-rp-subtle">Partial total</span
                    >{/if}
                </TimingCell>{/if}
            </td>
          {:else if column.key === 'tools'}
            <td
              class="table-cell text-left tabular-nums"
              title="Recorded tool calls across all repetitions"
            >
              {@render toolCount(group.summary.calls, group.summary.failures)}
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
      {#if attempts.length && viewKey === 'cost'}
        <tr aria-label="Estimated cost for shown attempts">
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
  .eval-results__rep-heading {
    @apply inline-flex flex-none items-center;
  }
  .eval-results__assessment {
    @apply flex items-center gap-1 whitespace-nowrap;
  }
  .eval-results__attempt :global(.table-cell),
  .eval-results__attempt :global(.eval-status) {
    color: var(--color-subtle);
  }
  .eval-results__case-cell {
    padding: 0 0.5rem;
    height: 1px;
  }
  .eval-results__case-cell :global(.tree-row-identity) {
    height: 100%;
  }
  .eval-results__rep-label {
    @apply min-h-[var(--signal-card-row-h)] text-left text-sm font-normal;
    color: var(--color-subtle);
  }
</style>

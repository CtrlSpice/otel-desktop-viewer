<script lang="ts">
  import type { ComponentProps } from 'svelte'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import {
    CheckListIcon,
    Timer02Icon,
    Coins01Icon,
  } from '@hugeicons/core-free-icons'
  import PaneHeader, { paneTabID } from '@/components/shared/PaneHeader.svelte'
  import AssessmentBadge from './AssessmentBadge.svelte'
  import RunStatusBadge from './RunStatusBadge.svelte'
  import EvalStatus from './EvalStatus.svelte'
  import { caseSummary } from './case-summary'
  import TableSortButton from '@/components/shared/TableSortButton.svelte'
  import TableColumnHeading from '@/components/shared/TableColumnHeading.svelte'
  import ResizableTable from '@/components/shared/ResizableTable.svelte'
  import TreeConnector from '@/components/shared/TreeConnector.svelte'
  import TreeToggle from '@/components/shared/TreeToggle.svelte'
  import TreeGutter from '@/components/shared/TreeGutter.svelte'
  import TreeChildCount from '@/components/shared/TreeChildCount.svelte'
  import LegendRows from '@/components/shared/LegendRows.svelte'
  import CorrelatedSpanBar from '@/components/shared/CorrelatedSpanBar.svelte'
  import DurationBar from '@/components/shared/DurationBar.svelte'
  import { telemetryAPI } from '@/services/telemetry-service'
  import type { SpanTiming } from '@/types/api-types'
  import { formatDuration } from '@/utils/time'
  import { completionTone, groupAttempts } from './group-attempts'
  import {
    cost,
    estimatedCost,
    percent,
    traceHref,
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
    onSelect,
  }: {
    run: EvalRun
    attempts: EvalAttempt[]
    selectedID?: string
    onSelect: (id: string) => void
  } = $props()
  const panelID = $props.id()
  const views = [
    { id: 'results', label: 'Results' },
    { id: 'time', label: 'Time' },
    { id: 'cost', label: 'Cost' },
  ]
  let view = $state('results')
  const viewLabel = $derived(
    views.find(item => item.id === view)?.label ?? 'Results'
  )
  type LegendEntry = Pick<ComponentProps<typeof EvalStatus>, 'label' | 'tone'>
  const scoreLegend: LegendEntry[] = [
    { label: '100%', tone: 'success' },
    { label: 'Partial', tone: 'warning' },
    { label: '0%', tone: 'error' },
    { label: 'Error', tone: 'papaya' },
    { label: 'Unscored', tone: 'neutral' },
  ]
  const completionLegend: LegendEntry[] = [
    { label: 'Complete', tone: 'complete' },
    { label: 'Running', tone: 'running' },
    { label: 'Planned', tone: 'neutral' },
    { label: 'Error', tone: 'papaya' },
  ]
  let caseGutterWidth = $state(0)
  let repGutterWidth = $state(0)
  let collapsedCases = $state<string[]>([])
  let expandedAttempts = $state<string[]>([])
  let spanTimings = $state<Record<string, SpanTiming>>({})
  let timingsLoading = $state(false)
  let timingsFailed = $state(false)
  const timingKey = (reference: TelemetryReference) =>
    `${reference.traceID.toLowerCase()}/${reference.spanID?.toLowerCase() ?? ''}`
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
    if (view !== 'time') return
    const references = [
      run.telemetry,
      ...run.attempts.flatMap(attempt => [
        attempt.telemetry,
        ...attempt.tools.map(tool => tool.telemetry),
      ]),
    ].flatMap(reference =>
      reference?.spanID
        ? [{ traceID: reference.traceID, spanID: reference.spanID }]
        : []
    )
    const controller = new AbortController()
    spanTimings = {}
    timingsLoading = true
    timingsFailed = false
    void telemetryAPI.getSpanTimings(references, controller.signal).then(
      timings => {
        if (controller.signal.aborted) return
        spanTimings = Object.fromEntries(
          timings.map(span => [timingKey(span), span])
        )
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
      width: Math.max(256, repGutterWidth),
      min: Math.max(144, repGutterWidth),
      numeric: true,
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
      key: 'seconds',
      label: 'Duration',
      hint: runWindow
        ? 'Recorded starts and durations on one timeline across this run'
        : 'Recorded starts within each question, with one duration scale across questions',
      group: 'Time',
      width: 288,
      min: 224,
    },
    {
      key: 'tools',
      label: 'Tool calls',
      group: 'Time',
      width: 112,
      min: 104,
    },
    {
      key: 'failures',
      label: 'Failures',
      hint: 'Failed tool calls',
      group: 'Time',
      width: 104,
      min: 96,
    },
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
      column => column.group === 'Case' || column.group === viewLabel
    )
  )
  let sorts = $state<Record<string, AttemptSort | undefined>>({})
  const activeSort = $derived(sorts[view])
  const measuredAttempts = $derived(
    run.attempts.map(attempt => {
      const duration = durationFor(attempt.telemetry, attempt.seconds)
      return {
        ...attempt,
        seconds: duration !== undefined ? Number(duration) / 1e9 : undefined,
      }
    })
  )
  const groups = $derived(
    groupAttempts(
      view === 'time'
        ? measuredAttempts.filter(item =>
            attempts.some(attempt => attempt.id === item.id)
          )
        : attempts,
      view === 'time' ? measuredAttempts : run.attempts,
      activeSort
    ).map(group => ({
      ...group,
      summary: caseSummary(group.allAttempts, run.assessments),
      duration: group.allAttempts.reduce((total, attempt) => {
        return total + (durationFor(attempt.telemetry, attempt.seconds) ?? 0n)
      }, 0n),
    }))
  )

  function toggleSort(key: AttemptSortKey) {
    sorts[view] = {
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

{#snippet spanBar(
  reference?: TelemetryReference,
  origin?: bigint,
  seconds?: number
)}
  {@const span = timingFor(reference)}
  {@const duration = durationFor(reference, seconds)}
  {#if span && span.endTime >= span.startTime}<CorrelatedSpanBar
      {span}
      maximum={maximumDuration}
      href={traceHref(span)}
      {origin}
    />
  {:else if duration !== undefined}
    <span
      class="block"
      title="Recorded eval duration"
      style:--bar-color="currentColor"
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
{/snippet}

{#snippet resultsIcon()}<HugeiconsIcon
    icon={CheckListIcon}
    size={16}
    strokeWidth={1.5}
    aria-hidden="true"
  />{/snippet}
{#snippet timeIcon()}<HugeiconsIcon
    icon={Timer02Icon}
    size={16}
    strokeWidth={1.5}
    aria-hidden="true"
  />{/snippet}
{#snippet costIcon()}<HugeiconsIcon
    icon={Coins01Icon}
    size={16}
    strokeWidth={1.5}
    aria-hidden="true"
  />{/snippet}

<PaneHeader
  pill
  mode="title-tabs"
  title={run.name}
  tabs={views.map(item => ({
    ...item,
    icon:
      item.id === 'results'
        ? resultsIcon
        : item.id === 'time'
          ? timeIcon
          : costIcon,
  }))}
  activeID={view}
  onSelect={id => (view = id)}
  tabLayout="right"
  tabPanelID={panelID}
>
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
  {#snippet metaLeft()}
    <LegendRows
      items={view === 'results' ? scoreLegend : completionLegend}
      label={view === 'results'
        ? 'Assessment score legend'
        : 'Execution status legend'}
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
  role="tabpanel"
  aria-labelledby={paneTabID(panelID, view)}
  class="flex min-h-0 flex-1 flex-col"
  tabindex="0"
>
  <ResizableTable
    columns={columns.map(column => ({
      id: column.key,
      label: column.label,
      min: column.min,
      width: column.width,
      fixed: column.fixed,
    }))}
    storageKey={`evals-${view}-column-widths`}
    caption={`${viewLabel} for ${run.name}`}
    pinnedCount={2}
    fillColumn="repetition"
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
                direction={activeSort?.key === column.key
                  ? activeSort.direction
                  : undefined}
                align={column.numeric ? 'right' : 'left'}
                title={column.hint}
                onclick={() => toggleSort(column.key)}
              >
                <TableColumnHeading
                  label={column.label}
                  description={column.description}
                  align={column.numeric ? 'right' : 'left'}
                />
              </TableSortButton>
            </th>
          {/if}
        {/each}
      </tr>
    {/snippet}
    {#snippet children(layout)}
      {#if view === 'time' && (run.telemetry || run.seconds !== undefined)}
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
          <td class="table-cell"
            >{@render spanBar(
              run.telemetry,
              runWindow?.startTime,
              run.seconds
            )}</td
          >
          <td class="table-cell"></td><td class="table-cell"></td>
        </tr>
      {/if}
      {#each groups as group (group.id)}
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
            <div
              class="eval-results__case-name"
              title={`${group.name} — ${group.state}`}
            >
              <TreeToggle
                label={`${collapsedCases.includes(group.id) ? 'Expand' : 'Collapse'} ${group.name}, ${group.attempts.length} ${group.attempts.length === 1 ? 'repetition' : 'repetitions'}`}
                expanded={!collapsedCases.includes(group.id)}
                onclick={() => toggleCase(group.id)}
                ><TreeGutter
                  depth={0}
                  tree={{
                    childrenCount: group.attempts.length,
                    ancestorHasNextSibling: [],
                    isLastChild: true,
                  }}
                  subtreeCollapsed={collapsedCases.includes(group.id)}
                >
                  {#snippet node()}<EvalStatus
                      label={group.state}
                      dotOnly
                      tone={completionTone(group.state)}
                    />{/snippet}
                </TreeGutter>
                <TreeChildCount count={group.attempts.length} />
              </TreeToggle>
              <span class="min-w-0 [overflow-wrap:anywhere]">{group.name}</span>
            </div>
          </th>
          {@render totals(group)}
        </tr>
        {#if !collapsedCases.includes(group.id)}
          {#each group.attempts as item, index (item.id)}
            <tr
              class="table-row eval-results__attempt"
              data-attempt-id={item.id}
              data-case-id={group.id}
              class:table-row--selected={selectedID === item.id}
            >
              {#each columns as column (column.key)}
                {#if column.key === 'name' && view === 'time'}
                  <th
                    colspan="2"
                    scope="row"
                    class="table-cell eval-results__case-cell font-normal"
                    class:table-cell--pinned={layout.pinned.name !== undefined}
                    style:left={layout.pinned.name === undefined
                      ? undefined
                      : `${layout.pinned.name}px`}
                  >
                    <div class="eval-results__case-name">
                      {#if item.tools.length}
                        <TreeToggle
                          label={`${expandedAttempts.includes(item.id) ? 'Collapse' : 'Expand'} tools for ${item.name}, repetition ${item.repetition}, ${item.tools.length} tool calls`}
                          expanded={expandedAttempts.includes(item.id)}
                          onclick={() =>
                            (expandedAttempts = expandedAttempts.includes(
                              item.id
                            )
                              ? expandedAttempts.filter(id => id !== item.id)
                              : [...expandedAttempts, item.id])}
                        >
                          <TreeGutter
                            depth={1}
                            tree={{
                              childrenCount: item.tools.length,
                              ancestorHasNextSibling: [true],
                              isLastChild: index === group.attempts.length - 1,
                            }}
                            subtreeCollapsed={!expandedAttempts.includes(
                              item.id
                            )}
                          >
                            {#snippet node()}<EvalStatus
                                label={item.state}
                                dotOnly
                                tone={completionTone(item.state)}
                              />{/snippet}
                          </TreeGutter>
                          <TreeChildCount count={item.tools.length} />
                        </TreeToggle>
                      {:else}
                        <TreeGutter
                          depth={1}
                          tree={{
                            childrenCount: 0,
                            ancestorHasNextSibling: [true],
                            isLastChild: index === group.attempts.length - 1,
                          }}
                          subtreeCollapsed={false}
                        >
                          {#snippet node()}<EvalStatus
                              label={item.state}
                              dotOnly
                              tone={completionTone(item.state)}
                            />{/snippet}
                        </TreeGutter>
                      {/if}
                      <button
                        class="eval-results__rep-button"
                        aria-label={item.name}
                        aria-pressed={selectedID === item.id}
                        onclick={() => onSelect(item.id)}
                        ><span class="sr-only">{item.name}</span>Rep {item.repetition}</button
                      >
                    </div>
                  </th>
                {:else if column.key === 'repetition' && view === 'time'}
                  <!-- Time's tree identity spans both Case and Rep columns. -->
                {:else if column.key === 'name'}
                  <td
                    class="table-cell eval-results__branch-cell"
                    class:table-cell--pinned={layout.pinned.name !== undefined}
                    style:left={layout.pinned.name === undefined
                      ? undefined
                      : `${layout.pinned.name}px`}
                  >
                    <TreeConnector
                      kind={index === group.attempts.length - 1
                        ? 'elbow'
                        : 'tee'}
                      start="calc(0.5rem + var(--waterfall-gutter-col) / 2)"
                      reach="calc(100% - 0.5rem - var(--waterfall-gutter-col) / 2)"
                      targetSize="0px"
                    />
                    <span class="sr-only">{item.name}</span>
                  </td>
                {:else if column.key === 'repetition'}
                  <th
                    scope="row"
                    class="table-cell text-right font-normal tabular-nums"
                    class:table-cell--pinned={layout.pinned.repetition !==
                      undefined}
                    style:left={layout.pinned.repetition === undefined
                      ? undefined
                      : `${layout.pinned.repetition}px`}
                  >
                    <span
                      class="eval-results__rep"
                      style:width={`calc(${repGutterWidth}px - 1rem)`}
                    >
                      <span class="eval-results__rep-arm"
                        ><TreeConnector
                          kind="arm"
                          start="0px"
                          reach="100%"
                          targetSize="0px"
                        /></span
                      >
                      <button
                        class="eval-results__rep-button"
                        title={`${item.name} — Rep ${item.repetition} · ${item.state}`}
                        aria-label={item.name}
                        aria-describedby={`${panelID}-status-${item.id}`}
                        aria-pressed={selectedID === item.id}
                        onclick={() => onSelect(item.id)}
                        >{item.repetition}</button
                      >
                      <span id={`${panelID}-status-${item.id}`} class="sr-only"
                        >Rep {item.repetition} · {item.state}</span
                      >
                    </span>
                  </th>
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
                  <td class="table-cell text-left tabular-nums">
                    {@render spanBar(
                      item.telemetry,
                      runWindow?.startTime ??
                        attemptWindows[item.id]?.startTime,
                      item.seconds
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
            {#if view === 'time' && expandedAttempts.includes(item.id)}
              {#each item.tools as tool, toolIndex}
                <tr
                  class="table-row eval-results__attempt"
                  data-tool-attempt-id={item.id}
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
                    <div class="eval-results__case-name" title={tool.message}>
                      <TreeGutter
                        depth={2}
                        tree={{
                          childrenCount: 0,
                          ancestorHasNextSibling: [
                            index < group.attempts.length - 1,
                            true,
                          ],
                          isLastChild: toolIndex === item.tools.length - 1,
                        }}
                        subtreeCollapsed={false}
                      >
                        {#snippet node()}<EvalStatus
                            label={tool.state}
                            dotOnly
                            tone={tool.state === 'Error' ? 'error' : 'complete'}
                          />{/snippet}
                      </TreeGutter>
                      <span class="min-w-0 [overflow-wrap:anywhere]"
                        >{tool.name}</span
                      >
                    </div>
                  </th>
                  <td class="table-cell"
                    >{@render spanBar(
                      tool.telemetry,
                      runWindow?.startTime ??
                        attemptWindows[item.id]?.startTime,
                      tool.seconds
                    )}</td
                  >
                  <td class="table-cell"></td><td class="table-cell"></td>
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
              class="table-cell text-left tabular-nums"
              title="Sum of recorded question durations across all repetitions"
            >
              {group.summary.duration.total === null
                ? ''
                : formatDuration(group.duration)}
              {#if group.summary.duration.incomplete && group.summary.duration.total !== null}<span
                  class="block text-rp-subtle">Partial total</span
                >{/if}
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
    min-width: var(--waterfall-gutter-col);
  }
  .eval-results__rep-heading {
    @apply block pr-4;
    min-width: var(--waterfall-gutter-col);
  }
  .eval-results__assessment {
    @apply flex items-center gap-1 whitespace-nowrap;
  }
  .eval-results__attempt :global(.table-cell),
  .eval-results__attempt :global(.eval-status) {
    color: var(--color-subtle);
  }
  .eval-results__branch-cell {
    position: relative;
  }
  .eval-results__case-name {
    @apply relative flex min-h-[var(--table-row-h)] w-full items-center gap-1 px-2 text-left font-normal;
  }
  .eval-results__case-cell {
    padding: 0;
  }
  .eval-results__rep {
    @apply -ml-4 flex items-center gap-2;
  }
  .eval-results__rep-arm {
    @apply relative h-[var(--signal-card-row-h)] flex-1;
  }
  .eval-results__rep-button {
    @apply min-h-[var(--signal-card-row-h)] shrink-0 rounded-sm text-right text-sm font-normal focus-visible:outline-2 focus-visible:outline-primary;
    color: var(--color-subtle);
  }
</style>

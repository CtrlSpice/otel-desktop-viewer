<script lang="ts">
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
  import { completionTone, groupAttempts } from './group-attempts'
  import {
    cost,
    estimatedCost,
    percent,
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
  let caseGutterWidth = $state(0)
  let repGutterWidth = $state(0)
  let collapsedCases = $state<string[]>([])
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
      min: Math.max(256, repGutterWidth),
      fixed: true,
      numeric: true,
    },
    ...run.assessments.map(assessment => ({
      key: `assessment:${assessment.id}` as AttemptSortKey,
      label: assessment.name,
      group: 'Results',
      width: 192,
      min: 176,
      hint: assessment.scale,
      assessment,
    })),
    {
      key: 'seconds',
      label: 'Duration',
      group: 'Time',
      width: 128,
      min: 112,
      numeric: true,
    },
    {
      key: 'tools',
      label: 'Tool calls',
      group: 'Time',
      width: 160,
      min: 144,
      numeric: true,
    },
    {
      key: 'investigatorCost',
      label: 'Investigator',
      group: 'Cost',
      width: 176,
      min: 160,
      numeric: true,
      description: 'Estimate · CAD',
    },
    {
      key: 'graderCost',
      label: 'Grader',
      group: 'Cost',
      width: 176,
      min: 160,
      numeric: true,
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
  const groups = $derived(
    groupAttempts(attempts, run.attempts, activeSort).map(group => ({
      ...group,
      summary: caseSummary(group.allAttempts, run.assessments),
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
              {#if !collapsedCases.includes(group.id)}<TreeConnector
                  kind="stem"
                  start="calc(1rem + 4px)"
                  targetSize="8px"
                />{/if}
              <TreeToggle
                label={`${collapsedCases.includes(group.id) ? 'Expand' : 'Collapse'} ${group.name}, ${group.attempts.length} ${group.attempts.length === 1 ? 'repetition' : 'repetitions'}`}
                expanded={!collapsedCases.includes(group.id)}
                onclick={() => toggleCase(group.id)}
                ><EvalStatus
                  label={group.state}
                  dotOnly
                  tone={completionTone(group.state)}
                />
                <span class="eval-results__case-count"
                  >{group.attempts.length}</span
                >
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
                {#if column.key === 'name'}
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
                      start="calc(1rem + 4px)"
                      reach="calc(100% - 1rem - 4px)"
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
                  <td class="table-cell text-right tabular-nums">
                    {#if item.seconds !== undefined}
                      <span>{item.seconds} s</span>
                    {:else}Unknown{/if}
                  </td>
                {:else if column.key === 'tools'}
                  {@const failed = item.tools.filter(
                    tool => tool.state === 'Error'
                  ).length}
                  <td class="table-cell text-right tabular-nums">
                    <span class="whitespace-nowrap"
                      >{item.tools.length}{#if failed}{' '}({failed}
                        {failed === 1 ? 'failure' : 'failures'}){/if}</span
                    >
                  </td>
                {:else if column.key === 'investigatorCost' || column.key === 'graderCost'}
                  <td class="table-cell text-right tabular-nums"
                    >{cost(item[column.key])}</td
                  >
                {/if}
              {/each}
            </tr>
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
              class="table-cell text-right tabular-nums"
              title="Sum of recorded repetition durations"
            >
              {group.summary.duration.total === null
                ? 'Unknown'
                : `${Number(group.summary.duration.total.toFixed(3))} s`}
              {#if group.summary.duration.incomplete && group.summary.duration.total !== null}<span
                  class="block text-rp-subtle">Partial total</span
                >{/if}
            </td>
          {:else if column.key === 'tools'}
            <td
              class="table-cell text-right tabular-nums"
              title="Recorded tool calls across all repetitions"
            >
              <span class="whitespace-nowrap"
                >{group.summary.calls}{#if group.summary.failures}{' '}
                  ({group.summary.failures}
                  {group.summary.failures === 1
                    ? 'failure'
                    : 'failures'}){/if}</span
              >
            </td>
          {:else if column.key === 'investigatorCost' || column.key === 'graderCost'}
            {@const estimate = group.summary[column.key]}
            <td class="table-cell text-right tabular-nums">
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
                class="table-cell text-right text-sm font-normal tabular-nums"
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
    @apply relative flex min-h-[var(--table-row-h)] w-full items-center gap-2 px-4 text-left font-normal;
  }
  .eval-results__case-cell {
    padding: 0;
  }
  .eval-results__case-count {
    width: var(--waterfall-gutter-col);
    @apply shrink-0 text-center tabular-nums;
    color: var(--color-subtle);
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

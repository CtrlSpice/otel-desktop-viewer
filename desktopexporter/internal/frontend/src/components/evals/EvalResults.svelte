<script lang="ts">
  import PaneHeader, { paneTabID } from '@/components/shared/PaneHeader.svelte'
  import AssessmentBadge from './AssessmentBadge.svelte'
  import RunStatusBadge from './RunStatusBadge.svelte'
  import EvalStatus from './EvalStatus.svelte'
  import { cost, estimatedCost, type EvalAttempt, type EvalRun } from './model'

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
  const longest = $derived(
    Math.max(0, ...run.attempts.map(item => item.seconds ?? 0))
  )
  const investigator = $derived(estimatedCost(attempts, 'investigatorCost'))
  const grader = $derived(estimatedCost(attempts, 'graderCost'))
</script>

<PaneHeader
  mode="title-tabs"
  title={run.name}
  subtitle={run.evaluation}
  tabs={views}
  activeID={view}
  onSelect={id => (view = id)}
  tabLayout="left"
  tabPanelID={panelID}
>
  {#snippet badge()}
    <span
      class="badge-count tabular-nums"
      title="Shown attempts / total attempts"
      >{attempts.length}/{run.attempts.length} attempts</span
    >
    <RunStatusBadge state={run.state} pauseReason={run.pauseReason} />
  {/snippet}
</PaneHeader>

<div
  id={panelID}
  role="tabpanel"
  aria-labelledby={paneTabID(panelID, view)}
  class="min-h-0 flex-1 overflow-auto"
  tabindex="0"
>
  <table class="eval-results table w-full min-w-[38rem]">
    <caption class="sr-only"
      >{views.find(item => item.id === view)?.label} for {run.name}</caption
    >
    <thead class="header-surface sticky top-0 z-10">
      <tr>
        <th class="table-cell min-w-56 text-left" scope="col">Case / attempt</th
        >
        {#if view === 'results'}
          {#each run.assessments as definition}
            <th
              class="table-cell min-w-32 text-right"
              scope="col"
              title={definition.scale}>{definition.name}</th
            >
          {/each}
        {:else if view === 'time'}
          <th class="table-cell min-w-40 text-right" scope="col">Duration</th>
          <th class="table-cell min-w-32 text-right" scope="col">Tool calls</th>
        {:else}
          <th class="table-cell min-w-40 text-right" scope="col"
            >Investigator <span class="block text-sm font-normal"
              >Estimate · CAD</span
            ></th
          >
          <th class="table-cell min-w-40 text-right" scope="col"
            >Grader <span class="block text-sm font-normal">Estimate · CAD</span
            ></th
          >
        {/if}
      </tr>
    </thead>
    <tbody class="table-body-surface">
      {#each attempts as item (item.id)}
        <tr
          class="table-row"
          class:table-row--selected={selectedID === item.id}
        >
          <th scope="row" class="table-cell text-left font-normal">
            <button
              class="block min-h-[var(--signal-card-row-h)] w-full rounded-sm text-left text-sm font-normal focus-visible:outline-2 focus-visible:outline-primary"
              aria-pressed={selectedID === item.id}
              onclick={() => onSelect(item.id)}>{item.name}</button
            >
            <div
              class="flex min-h-[var(--signal-card-row-h)] flex-wrap items-center gap-x-2 text-sm text-rp-subtle"
            >
              <EvalStatus
                label={item.state === 'Error' ? 'Execution error' : item.state}
                tone={item.state === 'Error' ? 'papaya' : undefined}
              />
              <span aria-hidden="true">·</span>
              <span>Rep {item.repetition}</span>
            </div>
          </th>
          {#if view === 'results'}
            {#each run.assessments as definition}
              {@const assessment = item.assessments.find(
                score => score.id === definition.id
              )}
              <td class="table-cell text-right">
                <div class="flex flex-col items-end gap-0">
                  <AssessmentBadge {assessment} maximum={definition.maximum} />
                  {#if assessment?.value != null}<span
                      class="min-h-[var(--signal-card-row-h)] content-center text-sm tabular-nums"
                      >{assessment.value} / {definition.maximum}</span
                    >{/if}
                </div>
              </td>
            {/each}
          {:else if view === 'time'}
            {@const failed = item.tools.filter(
              tool => tool.state === 'Error'
            ).length}
            <td class="table-cell text-right tabular-nums">
              {#if item.seconds !== undefined}
                <span class="text-sm">{item.seconds} s</span>
                <div
                  class="mt-2 h-1 overflow-hidden rounded-full bg-base-300"
                  aria-hidden="true"
                >
                  <div
                    class="h-full rounded-full bg-primary"
                    style:width="{longest > 0
                      ? (item.seconds / longest) * 100
                      : 0}%"
                  ></div>
                </div>
              {:else}<span class="text-sm">Unknown</span>{/if}
            </td>
            <td class="table-cell text-right text-sm tabular-nums">
              <div class="min-h-[var(--signal-card-row-h)] content-center">
                {item.tools.length}
              </div>
              {#if failed}<div
                  class="min-h-[var(--signal-card-row-h)] content-center"
                >
                  <EvalStatus label={`${failed} failed`} tone="error" />
                </div>{/if}
            </td>
          {:else}
            <td class="table-cell text-right text-sm tabular-nums"
              >{cost(item.investigatorCost)}</td
            >
            <td class="table-cell text-right text-sm tabular-nums"
              >{cost(item.graderCost)}</td
            >
          {/if}
        </tr>
      {/each}
    </tbody>
    {#if view === 'cost' && attempts.length}
      <tfoot class="header-surface">
        <tr>
          <th scope="row" class="table-cell text-left text-sm font-normal"
            >Shown attempts</th
          >
          {#each [investigator, grader] as estimate}
            <td class="table-cell text-right text-sm tabular-nums"
              >{cost(
                estimate.total
              )}{#if estimate.incomplete && estimate.total !== null}<span
                  class="block text-sm text-rp-subtle">Incomplete</span
                >{/if}</td
            >
          {/each}
        </tr>
      </tfoot>
    {/if}
  </table>
</div>

<style lang="postcss">
  @reference "../../app.css";

  .eval-results th[scope='col'] {
    @apply font-normal;
    color: var(--color-subtle);
  }

  .eval-results tfoot {
    display: table-footer-group;
  }
</style>

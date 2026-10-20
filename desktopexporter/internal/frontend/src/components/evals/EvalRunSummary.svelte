<script lang="ts">
  import SignalCard from '@/components/shared/SignalCard.svelte'
  import OutcomeBar from '@/components/shared/OutcomeBar.svelte'
  import Identifier from '@/components/shared/Identifier.svelte'
  import RunStatusBadge from './RunStatusBadge.svelte'
  import { cost, percent, type EvalRun } from './model'
  import { runSummary } from './run-summary'

  let {
    run,
    selected = false,
    onclick,
  }: {
    run: EvalRun
    selected?: boolean
    onclick: (id: string) => void
  } = $props()
  const summary = $derived(runSummary(run))
</script>

{#snippet outcomeSummary()}
  <OutcomeBar
    parts={summary.parts}
    label={summary.definition?.name ?? 'Results'}
  >
    {#snippet right()}
      <span class="text-sm text-rp-subtle" title={run.id}>
        <span class="sr-only">Run ID: </span>
        <Identifier value={run.id} />
      </span>
    {/snippet}
  </OutcomeBar>
{/snippet}

<SignalCard
  id={run.id}
  title={run.name}
  interactiveSubtitle={outcomeSummary}
  {selected}
  {onclick}
>
  {#snippet badge()}<RunStatusBadge
      state={run.state}
      pauseReason={run.pauseReason}
    />{/snippet}
  {#snippet date()}
    <div
      class="flex min-h-[var(--signal-card-row-h)] flex-wrap items-center justify-between gap-x-2 gap-y-0 text-sm tabular-nums"
    >
      <span title="Completed investigations / planned attempts"
        >Assessed: <span class="text-base-content"
          >{summary.assessed}/{summary.planned}</span
        ></span
      >
      <span
        class="text-center"
        title="Attempts with a recorded headline score / planned attempts"
        >Graded: <span class="text-base-content"
          >{summary.assessment?.scored ?? 0}/{summary.planned}</span
        ></span
      >
      <span
        class="text-right"
        title={summary.assessment
          ? `${summary.definition?.name}: provisional mean across ${summary.assessment.denominator} ${summary.definition?.aggregation === 'planned' ? 'planned' : 'scored'} attempts`
          : 'No headline assessment defined'}
        >Score: <span class="text-base-content"
          >{percent(summary.assessment?.percent ?? null)}</span
        ></span
      >
    </div>
  {/snippet}
  {#snippet meta()}
    <div
      class="flex min-h-[var(--signal-card-row-h)] w-full flex-wrap items-center justify-between gap-x-2 gap-y-0 text-sm tabular-nums"
    >
      <span>
        Tool calls: <span class="text-base-content">{summary.toolCalls}</span>
        {#if summary.failedTools > 0}
          <span class="text-error">({summary.failedTools} failed)</span>
        {/if}
      </span>
      <span title="Investigator cost estimate in CAD"
        >Est. cost: {cost(
          summary.investigatorCost.total
        )}{#if summary.investigatorCost.incomplete && summary.investigatorCost.total !== null}
          {' · incomplete'}{/if}</span
      >
    </div>
  {/snippet}
</SignalCard>

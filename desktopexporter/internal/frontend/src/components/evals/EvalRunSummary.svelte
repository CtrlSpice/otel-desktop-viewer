<script lang="ts">
  import SignalCard from '@/components/shared/SignalCard.svelte'
  import OutcomeBar from '@/components/shared/OutcomeBar.svelte'
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

<SignalCard id={run.id} title={run.name} {selected} {onclick}>
  {#snippet badge()}<RunStatusBadge state={run.state} />{/snippet}
  {#snippet meta()}
    <div class="flex w-full min-w-0 flex-col gap-2 py-1">
      <div class="flex min-w-0 flex-col gap-2">
        <div class="w-full min-w-0">
          <OutcomeBar
            parts={summary.parts}
            label={summary.definition?.name ?? 'Results'}
          />
        </div>
        <div
          class="grid grid-cols-3 items-center gap-2 text-xs tabular-nums"
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
            >Score: <strong class="text-base-content"
              >{percent(summary.assessment?.percent ?? null)}</strong
            ></span
          >
        </div>
      </div>
      <div
        class="flex flex-wrap items-center justify-between gap-x-2 gap-y-1 text-xs tabular-nums"
      >
        <span title="Investigator cost estimate in CAD"
          >Est. cost: {cost(
            summary.investigatorCost.total
          )}{#if summary.investigatorCost.incomplete && summary.investigatorCost.total !== null}
            {' · incomplete'}{/if}</span
        >
      </div>
      <div
        class="flex flex-wrap items-center justify-between gap-x-2 gap-y-1 text-xs tabular-nums"
      >
        <span>Tool calls</span>
        <span class="flex items-center gap-1">
          <span class="badge-count">Total: {summary.toolCalls}</span>
          <span
            class="badge badge-xs badge-soft"
            class:badge-error={summary.failedTools > 0}
            >Failed: {summary.failedTools}</span
          >
        </span>
      </div>
    </div>
  {/snippet}
</SignalCard>

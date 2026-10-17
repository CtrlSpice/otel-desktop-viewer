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
        <span
          class="self-end text-xs tabular-nums"
          title={summary.assessment
            ? `${summary.definition?.name}: provisional mean across ${summary.assessment.denominator} ${summary.definition?.aggregation === 'planned' ? 'planned' : 'scored'} attempts`
            : 'No headline assessment defined'}
          >Score: <strong class="text-base-content"
            >{percent(summary.assessment?.percent ?? null)}</strong
          ></span
        >
      </div>
      <div
        class="flex flex-wrap items-center justify-between gap-x-2 gap-y-1 text-xs tabular-nums"
      >
        <span class="badge-count" title="Assessed attempts / planned attempts"
          >Assessed: {summary.assessment?.scored ?? 0}/{summary.planned}</span
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
        <span>Tool calls: {summary.toolCalls}</span>
        {#if summary.failedTools}
          <span class="badge badge-xs badge-soft badge-error"
            >Failed calls: {summary.failedTools}</span
          >
        {:else}<span>Failed calls: 0</span>{/if}
      </div>
    </div>
  {/snippet}
</SignalCard>

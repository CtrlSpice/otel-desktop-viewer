<script lang="ts">
  import SignalCard from '@/components/shared/SignalCard.svelte'
  import OutcomeBar from '@/components/shared/OutcomeBar.svelte'
  import RunStatusBadge from './RunStatusBadge.svelte'
  import { cost, localTimestamp, percent, type EvalRun } from './model'
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
  {#snippet lead()}
    <div class="min-w-0 text-rp-subtle">
      <div>{run.evaluation}</div>
      {#each run.configuration as field}
        <div class="break-words">
          <span class="signal-row__label">{field.name}:</span>
          {field.value}
        </div>
      {/each}
    </div>
  {/snippet}
  {#snippet meta()}
    <div class="flex w-full min-w-0 flex-col gap-2 py-2">
      <div class="flex flex-wrap items-baseline gap-x-2 gap-y-1">
        <span class="signal-row__label"
          >{summary.definition?.name ?? 'Assessment'}:</span
        >
        <strong
          class="tabular-nums text-base-content"
          title={summary.assessment
            ? `Provisional mean across ${summary.assessment.denominator} ${summary.definition?.aggregation === 'planned' ? 'planned' : 'scored'} attempts`
            : 'No headline assessment defined'}
          >{percent(summary.assessment?.percent ?? null)}</strong
        >
        <span class="badge-count ml-auto"
          >Assessed: {summary.assessment?.scored ?? 0} / {summary.planned} attempts</span
        >
      </div>
      <OutcomeBar
        parts={summary.parts}
        label={summary.definition?.name ?? 'Results'}
      />
      <div class="flex flex-wrap gap-x-2 text-xs tabular-nums">
        <span>Investigator: {summary.started} attempts</span>
        <span
          >Cost (est., CAD) {cost(
            summary.investigatorCost.total
          )}{#if summary.investigatorCost.incomplete && summary.investigatorCost.total !== null}
            · incomplete{/if}</span
        >
      </div>
      <div
        class="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs tabular-nums"
      >
        <span>{summary.toolCalls} recorded tool calls</span>
        {#if summary.failedTools}
          <span class="badge badge-xs badge-soft badge-error"
            >{summary.failedTools} failed {summary.failedTools === 1
              ? 'tool call'
              : 'tool calls'}</span
          >
        {:else}<span>0 failed tool calls</span>{/if}
      </div>
      {#if run.startedAt}<time
          class="text-xs tabular-nums text-rp-subtle"
          datetime={run.startedAt}
          title={run.startedAt}>{localTimestamp(run.startedAt)}</time
        >{/if}
    </div>
  {/snippet}
</SignalCard>

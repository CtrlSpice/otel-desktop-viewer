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

{#snippet outcomeSummary()}
  <OutcomeBar
    parts={summary.parts}
    label={summary.definition?.name ?? 'Results'}
  />
{/snippet}

{#snippet runFacts()}
  <dl class="eval-summary__facts">
    {#if run.state !== 'Complete' || summary.assessed !== summary.planned}<div
        class="eval-summary__fact"
        title="Completed investigations / planned attempts"
      >
        <dt class="sr-only">Progress:</dt>
        <dd>{summary.assessed}/{summary.planned} complete</dd>
      </div>{/if}
    {#if (summary.assessment?.scored ?? 0) !== summary.assessed}
      <div
        class="eval-summary__fact"
        title="Attempts with a recorded headline score / planned attempts"
      >
        <dt>Graded:</dt>
        <dd>{summary.assessment?.scored ?? 0}/{summary.planned}</dd>
      </div>
    {/if}
    <div
      class="eval-summary__fact"
      title={summary.assessment
        ? `${summary.definition?.name}: provisional mean across ${summary.assessment.denominator} ${summary.definition?.aggregation === 'planned' ? 'planned' : 'scored'} attempts`
        : 'No headline assessment defined'}
    >
      <dt>Score:</dt>
      <dd><span>{percent(summary.assessment?.percent ?? null)}</span></dd>
    </div>
  </dl>
  <dl class="eval-summary__facts">
    <div class="eval-summary__fact">
      <dt class="sr-only">Tool activity:</dt>
      <dd>
        <span
          >{summary.toolCalls}
          {summary.toolCalls === 1 ? 'tool call' : 'tool calls'}</span
        >
        {#if summary.failedTools > 0}
          <span
            >({summary.failedTools}
            {summary.failedTools === 1 ? 'failure' : 'failures'})</span
          >
        {/if}
      </dd>
    </div>
    <div
      class="eval-summary__fact eval-summary__fact--cost"
      title="Investigator cost estimate in CAD"
    >
      <dt>
        {summary.investigatorCost.incomplete &&
        summary.investigatorCost.total !== null
          ? 'Partial est. cost:'
          : 'Est. cost:'}
      </dt>
      <dd>{cost(summary.investigatorCost.total)}</dd>
    </div>
  </dl>
{/snippet}

<SignalCard
  id={run.id}
  title={run.name}
  interactiveSubtitle={run.state === 'Planned' ? undefined : outcomeSummary}
  meta={run.state === 'Planned' ? undefined : runFacts}
  {selected}
  {onclick}
>
  {#snippet badge()}<RunStatusBadge
      state={run.state}
      pauseReason={run.pauseReason}
    />{#if run.state === 'Planned'}<span class="text-rp-subtle tabular-nums"
        >{' '}({summary.planned})</span
      >{/if}{/snippet}
</SignalCard>

<style lang="postcss">
  @reference "../../app.css";

  .eval-summary__facts {
    @apply flex w-full min-w-0 flex-wrap gap-x-4 text-sm tabular-nums;
  }

  .eval-summary__fact {
    @apply flex min-w-0 max-w-full items-center gap-1;
    min-height: var(--signal-card-row-h);
  }

  .eval-summary__fact dt {
    @apply shrink-0;
  }

  .eval-summary__fact--cost {
    margin-left: auto;
  }

  .eval-summary__fact dd {
    @apply flex min-w-0 flex-wrap items-center gap-x-2 text-base-content;
  }
</style>

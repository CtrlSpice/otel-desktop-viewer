<script lang="ts">
  import SignalCard from '@/components/shared/SignalCard.svelte'
  import SignalDate from '@/components/shared/SignalDate.svelte'
  import OutcomeBar from '@/components/shared/OutcomeBar.svelte'
  import RunStatusBadge from './RunStatusBadge.svelte'
  import type { EvalRunCardData } from './run-summary'

  let {
    run,
    selected = false,
    onclick,
  }: {
    run: EvalRunCardData
    selected?: boolean
    onclick: (id: string) => void
  } = $props()
  const started = $derived(run.startedAt ? Date.parse(run.startedAt) : NaN)
</script>

{#snippet outcomeSummary()}
  <OutcomeBar
    parts={run.outcomes}
    label={run.outcomeLabel}
    onsegmentclick={() => onclick(run.id)}
  />
{/snippet}

{#snippet startDate()}
  <SignalDate label="Started" timestamp={BigInt(started) * 1_000_000n} />
{/snippet}

<SignalCard
  id={run.id}
  title={run.name}
  interactiveSubtitle={run.state === 'Planned' ? undefined : outcomeSummary}
  date={Number.isFinite(started) ? startDate : undefined}
  {selected}
  {onclick}
>
  {#snippet badge()}<RunStatusBadge
      state={run.state}
      pauseReason={run.pauseReason}
    />{#if run.state === 'Planned'}<span class="text-rp-subtle tabular-nums"
        >{' '}[{run.repCount}]</span
      >{/if}{/snippet}
</SignalCard>

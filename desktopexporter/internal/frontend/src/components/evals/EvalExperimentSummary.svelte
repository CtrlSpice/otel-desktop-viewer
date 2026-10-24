<script lang="ts">
  import SignalCard from '@/components/shared/SignalCard.svelte'
  import SignalDate from '@/components/shared/SignalDate.svelte'
  import OutcomeBar from '@/components/shared/OutcomeBar.svelte'
  import ExperimentStatusBadge from './ExperimentStatusBadge.svelte'
  import type { EvalExperimentCardData } from './experiment-summary'

  let {
    experiment,
    selected = false,
    onclick,
  }: {
    experiment: EvalExperimentCardData
    selected?: boolean
    onclick: (id: string) => void
  } = $props()
  const started = $derived(
    experiment.startedAt ? Date.parse(experiment.startedAt) : NaN
  )
</script>

{#snippet outcomeSummary()}
  <OutcomeBar parts={experiment.outcomes} label={experiment.outcomeLabel} />
{/snippet}

{#snippet startDate()}
  <SignalDate label="Started" timestamp={BigInt(started) * 1_000_000n} />
{/snippet}

<SignalCard
  id={experiment.id}
  title={experiment.name}
  subtitleContent={experiment.state === 'Planned' ? undefined : outcomeSummary}
  date={Number.isFinite(started) ? startDate : undefined}
  {selected}
  {onclick}
>
  {#snippet badge()}<ExperimentStatusBadge
      state={experiment.state}
      pauseReason={experiment.pauseReason}
    />{#if experiment.state === 'Planned'}<span
        class="text-subtle tabular-nums"
        >{' '}[{experiment.resultCount}]</span
      >{/if}{/snippet}
</SignalCard>

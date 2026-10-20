<script lang="ts">
  import SignalCard from '@/components/shared/SignalCard.svelte'
  import SignalDate from '@/components/shared/SignalDate.svelte'
  import OutcomeBar from '@/components/shared/OutcomeBar.svelte'
  import RunStatusBadge from './RunStatusBadge.svelte'
  import type { EvalRun } from './model'
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
  const started = $derived(run.startedAt ? Date.parse(run.startedAt) : NaN)
</script>

{#snippet outcomeSummary()}
  <OutcomeBar
    parts={summary.parts}
    label={summary.definition?.name ?? 'Results'}
  />
{/snippet}

<SignalCard
  id={run.id}
  title={run.name}
  interactiveSubtitle={run.state === 'Planned' ? undefined : outcomeSummary}
  {selected}
  {onclick}
>
  {#snippet date()}<SignalDate
      label="Started"
      timestamp={Number.isFinite(started)
        ? BigInt(started) * 1_000_000n
        : undefined}
      unavailable={run.state === 'Planned' ? 'Not started' : 'Unavailable'}
    />{/snippet}
  {#snippet badge()}<RunStatusBadge
      state={run.state}
      pauseReason={run.pauseReason}
    />{#if run.state === 'Planned'}<span class="text-rp-subtle tabular-nums"
        >{' '}[{summary.planned}]</span
      >{/if}{/snippet}
</SignalCard>

<script lang="ts">
  import HugeiconsIcon from './Icon.svelte'
  import {
    Alert02Icon,
    ChartAverageIcon,
    Clock01Icon,
    Coins01Icon,
  } from '@hugeicons/core-free-icons'
  import ModelRow from './ModelRow.svelte'
  import GraderRow from './GraderRow.svelte'
  import { averageCost, integer, type Presentation } from './model'

  let { data }: { data: Presentation } = $props()
  const warning = $derived(data.provisional || data.synthetic)
  const maxTime = $derived(
    Math.max(
      1,
      ...data.models.map(model => model.investigator.maxSeconds ?? 0),
      ...data.grader.models.map(model => model.maxSeconds ?? 0)
    )
  )
  const maxCost = $derived(
    Math.max(
      0.001,
      ...data.models.map(model => averageCost(model.investigator) ?? 0),
      ...data.grader.models.map(model => averageCost(model) ?? 0)
    )
  )
  const views = [
    { view: 'result', label: 'Results', icon: ChartAverageIcon },
    { view: 'time', label: 'Time', icon: Clock01Icon },
    { view: 'cost', label: 'Cost (est., CAD)', icon: Coins01Icon },
  ]
</script>

<main>
  <header>
    <div class="eyebrow">OpenTelemetry Desktop Viewer</div>
    <h1>Eval results</h1>
  </header>
  <div
    class="notice alert {warning
      ? 'alert-warning notice-warning'
      : 'alert-info'}"
  >
    {#if warning}<HugeiconsIcon
        icon={Alert02Icon}
        size={20}
        strokeWidth={1.5}
      />{/if}
    <strong
      >{data.synthetic
        ? 'Synthetic preview · not model results.'
        : data.provisional
          ? 'Provisional · awaiting human signoff.'
          : 'Recorded attempts signed off.'}</strong
    >
  </div>
  <div class="run-meta">
    <span>Run {data.date}</span>
    <span
      >{data.models.length} models · {data.tasksObserved}/{integer(
        data.tasksExpected
      )} tasks · {integer(data.repetitions)} repetitions</span
    >
    <span
      >{data.attempts}/{integer(data.expectedAttempts)} recorded attempts</span
    >
  </div>
  <div class="board-label">
    <span>Model</span>
    <div class="metric-heading">
      <div class="metric-selector">
        <div class="metrics" role="group" aria-label="Model metric">
          {#each views as item, index}
            <span class="metric-choice">
              {#if index}<span class="metric-separator" aria-hidden="true"
                  >|</span
                >{/if}
              <button
                type="button"
                data-view={item.view}
                aria-pressed={item.view === 'result'}
                ><HugeiconsIcon
                  icon={item.icon}
                  size={20}
                  strokeWidth={1.5}
                />{item.label}</button
              >
            </span>
          {/each}
        </div>
      </div>
      <span class="mean-heading">Mean</span>
      <span class="total-heading">Total</span>
      <span class="cost-heading">Cost/run</span>
    </div>
    <span class="runs-label">Runs</span><span aria-hidden="true"></span>
  </div>
  <section aria-label="Model results">
    {#each data.models as model}
      <ModelRow
        {model}
        tasksExpected={data.tasksExpected}
        repetitions={data.repetitions}
        {maxTime}
        {maxCost}
      />
    {/each}
    {#each data.grader.models as grader}<GraderRow
        {data}
        {grader}
        {maxTime}
        {maxCost}
      />{/each}
  </section>
  {#if data.pricing}<p class="footnote">
      API-price estimates · CAD · Exchange rate dated {data.pricing.fxDate}.
    </p>{/if}
</main>

<script lang="ts">
  import HugeiconsIcon from './Icon.svelte'
  import { ChevronDownIcon } from '@hugeicons/core-free-icons'
  import Badge from './Badge.svelte'
  import ResultBar from './ResultBar.svelte'
  import TaskCard from './TaskCard.svelte'
  import TimeMetric from './TimeMetric.svelte'
  import CostMetric from './CostMetric.svelte'
  import {
    averageCost,
    cost,
    integer,
    outcomeBreakdown,
    percent,
    type Model,
  } from './model'

  let {
    model,
    tasksExpected,
    repetitions,
    maxTime,
    maxCost,
  }: {
    model: Model
    tasksExpected: number | null
    repetitions: number | null
    maxTime: number
    maxCost: number
  } = $props()
  const result = $derived(model.result)
  const metrics = $derived(model.investigator)
  const expected = $derived(
    tasksExpected !== null && repetitions !== null
      ? tasksExpected * repetitions
      : 'Unknown'
  )
  const parts = $derived(outcomeBreakdown(result))
</script>

<details class="model" data-live-key={model.model}>
  <summary>
    <div class="model-heading">
      <span class="chevron" aria-hidden="true"
        ><HugeiconsIcon
          icon={ChevronDownIcon}
          size={20}
          strokeWidth={1.5}
        /></span
      >
      <div class="model-name">
        <strong>{model.name}</strong><span>{model.provider}</span><span
          class="model-id">{model.model}</span
        >
      </div>
    </div>
    <div class="metric result">
      <div class="grade-distribution">
        <ResultBar {parts} label="Results" />
      </div>
      <div class="result-mean">
        <strong aria-label="Mean: {percent(result.displayMean)}"
          >{percent(result.displayMean)}</strong
        >
      </div>
    </div>
    <TimeMetric value={metrics} max={maxTime} />
    <CostMetric average={averageCost(metrics)} max={maxCost} />
    <span class="coverage" aria-label="Runs: {result.attempts}/{expected}"
      >{result.attempts}/{expected}</span
    >
    <span aria-hidden="true"></span>
  </summary>
  <div class="expanded">
    <h2 class="sr-only">{model.name} task results</h2>
    <div class="model-meta">
      <div class="grade-counts">
        {#each parts as part}<Badge
            status={part.status}
            label="{integer(part.count)} {part.label}"
          />{:else}No results{/each}
      </div>
      <span
        >Investigator: {integer(metrics.attempts)} sessions · total cost (est., CAD)
        {cost(metrics.apiCostCad, metrics.estimateCoverage)}</span
      >
      <span
        >{integer(metrics.toolCalls)} tool calls · {integer(
          metrics.failedTools
        )} recorded failed tools</span
      >
    </div>
    <div class="tasks">
      {#each model.tasks as task}<TaskCard {task} {repetitions} />{/each}
    </div>
  </div>
</details>

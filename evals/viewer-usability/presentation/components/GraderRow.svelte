<script lang="ts">
  import Badge from './Badge.svelte'
  import TimeMetric from './TimeMetric.svelte'
  import CostMetric from './CostMetric.svelte'
  import GradingTable from './GradingTable.svelte'
  import {
    averageCost,
    cost,
    gradeLabel,
    integer,
    seconds,
    type Presentation,
    type Grader,
  } from './model'

  let {
    data,
    grader,
    maxTime,
    maxCost,
  }: {
    data: Presentation
    grader: Grader
    maxTime: number
    maxCost: number
  } = $props()
  const outputs = $derived(
    data.grader.outputs.filter(output => output.graderModel === grader.model)
  )
  const counts = $derived(
    [1, 0.5, 0].map(score => ({
      score,
      count: outputs.filter(
        output => !output.executionError && output.score === score
      ).length,
    }))
  )
  const errors = $derived(
    outputs.filter(output => output.executionError).length
  )
  const unscored = $derived(
    outputs.filter(output => !output.executionError && output.score === null)
      .length
  )
  const scored = $derived(counts.reduce((sum, value) => sum + value.count, 0))
</script>

{#snippet cell(title: string, value: string)}
  <div>
    <dt>{title}</dt>
    <dd>{value}</dd>
  </div>
{/snippet}

<section
  class="model grader-row"
  data-live-key="grader:{grader.model ?? grader.name}"
  aria-label="Grader {grader.model ?? grader.name}"
>
  <div class="grader-summary">
    <div class="model-name">
      <strong>Grader</strong><span>{grader.model ?? grader.name}</span>
    </div>
    <div class="metric result grade-breakdown">
      {#if scored}
        <div class="grade-overview">
          <div class="grade-distribution">
            <div
              class="grade-strip"
              role="img"
              aria-label="Automated grades: {counts
                .map(value => `${value.count} ${gradeLabel(value.score)}`)
                .join(' · ')}"
            >
              {#each counts as value}<span
                  class="result-{value.score === 1
                    ? 'complete'
                    : value.score === 0.5
                      ? 'partial'
                      : 'no-credit'}"
                  style="flex:{value.count}"
                ></span>{/each}
            </div>
            <div class="grade-counts">
              {#each counts as value}<Badge
                  status={value.score === 1
                    ? 'complete'
                    : value.score === 0.5
                      ? 'partial'
                      : 'no-credit'}
                  label="{integer(value.count)} {gradeLabel(value.score)}"
                />{/each}
            </div>
          </div>
          <span class="grader-mean">sometimes, but<br />it means well</span>
        </div>
      {:else}<span>Unscored</span>{/if}
      {#if errors}<Badge
          status="execution-error"
          label={`${integer(errors)} execution ${errors === 1 ? 'error' : 'errors'}`}
        />{/if}
      {#if unscored}<span>{unscored} unscored</span>{/if}
      {#if grader.attempts !== null && outputs.length !== grader.attempts}<span
          >{outputs.length}/{grader.attempts} outputs recorded</span
        >{/if}
    </div>
    <TimeMetric value={grader} max={maxTime} />
    <CostMetric average={averageCost(grader)} max={maxCost} />
    <span
      class="coverage"
      aria-label="Grading runs: {integer(grader.attempts)}/{integer(
        data.expectedAttempts
      )}">{integer(grader.attempts)}/{integer(data.expectedAttempts)}</span
    >
    <span aria-hidden="true"></span>
  </div>
  <div class="expanded">
    <!-- svelte-ignore a11y_no_noninteractive_tabindex (keyboard users must be able to scroll this region) -->
    <div
      class="grader-metrics-scroll"
      data-grading-scroll-key="grader-summary"
      role="region"
      aria-label="Grading summary"
      tabindex="0"
    >
      <dl class="grader-metrics">
        {@render cell('Sessions', integer(grader.attempts))}
        {@render cell('Median time', seconds(grader.medianSeconds))}
        {@render cell('P95 time', seconds(grader.p95Seconds))}
        {@render cell('Input tokens', integer(grader.inputTokens))}
        {@render cell('Output tokens', integer(grader.outputTokens))}
        {@render cell(
          'Total cost (est., CAD)',
          cost(grader.apiCostCad, grader.estimateCoverage)
        )}
        {#if data.grader.executionErrors}
          <div>
            <dt>Execution errors</dt>
            <dd>
              <Badge
                status="execution-error"
                label={integer(data.grader.executionErrors)}
              />
            </dd>
          </div>
        {/if}
      </dl>
    </div>
    <GradingTable {data} {grader} />
  </div>
</section>

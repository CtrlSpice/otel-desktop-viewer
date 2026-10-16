<script lang="ts">
  import Badge from './Badge.svelte'
  import ResultBar from './ResultBar.svelte'
  import {
    cost,
    integer,
    outcomeBreakdown,
    runResult,
    seconds,
    taskName,
    type Task,
  } from './model'

  let { task, repetitions }: { task: Task; repetitions: number | null } =
    $props()
  const checks = $derived(task.checks)
  const corrections = $derived(
    task.runResults.filter(
      run => run.signedOff && run.automatedScore !== run.score
    )
  )
  const hasReview = $derived(
    task.signedAttempts ||
      corrections.length ||
      task.claimReviewedAttempts ||
      checks.artifactPass + checks.artifactFail
  )
</script>

{#snippet cell(title: string, value: string)}
  <div>
    <dt>{title}</dt>
    <dd>{value}</dd>
  </div>
{/snippet}

<article class="task card bg-base-200">
  <div class="task-title pane-header">
    <h3>{taskName(task.task)}</h3>
    {#if repetitions !== 1}
      <span
        class="task-count"
        aria-label="Runs: {task.attempts}/{integer(repetitions)}"
        >{task.attempts}/{integer(repetitions)}</span
      >
    {/if}
  </div>
  <div class="task-group result-group">
    <ResultBar parts={outcomeBreakdown(task)} label="Attempt results" task />
  </div>
  {#if hasReview}
    <div class="task-group">
      <dl>
        {#if task.signedAttempts}
          {@render cell(
            'Human signoff',
            task.attempts === 1
              ? 'Signed off'
              : `${task.signedAttempts}/${task.attempts} runs signed`
          )}
        {/if}
        {#each corrections as run}
          {@render cell(
            `Human correction${task.attempts > 1 ? ` · Run ${run.run}` : ''}`,
            `Automated ${run.automatedScore === null ? 'unscored' : runResult(run.automatedScore)} → signed ${runResult(run.score!)}`
          )}
        {/each}
        {#if task.claimReviewedAttempts}
          {@render cell(
            'Human claim review',
            `${task.supportedClaims} supported · ${task.contradictedClaims} contradicted · ${task.unverifiedClaims} unverified`
          )}
        {/if}
        {#if checks.artifactPass + checks.artifactFail}
          {@render cell(
            'Artifact check',
            checks.artifactPass + checks.artifactFail === 1
              ? checks.artifactPass
                ? 'Passed'
                : 'Failed'
              : [
                  [checks.artifactPass, 'passed'],
                  [checks.artifactFail, 'failed'],
                ]
                  .filter(([count]) => Number(count) > 0)
                  .map(([count, label]) => `${count} ${label}`)
                  .join(' · ')
          )}
        {/if}
      </dl>
    </div>
  {/if}
  <div class="task-group">
    <dl>
      {@render cell('Median time', seconds(task.medianSeconds))}
      {#if task.attempts > 1}{@render cell(
          'P95 time',
          seconds(task.p95Seconds)
        )}{/if}
      {#if task.timedAttempts !== task.attempts}{@render cell(
          'Time coverage',
          `${task.timedAttempts}/${task.attempts} attempts`
        )}{/if}
      {@render cell('Tool calls', integer(task.toolCalls))}
      {@render cell('Failed tool calls', integer(task.failedTools))}
      <div>
        <dt>Execution errors</dt>
        <dd>
          {#if task.error}<Badge
              status={task.harnessError === task.error
                ? 'harness-error'
                : 'execution-error'}
              label={integer(task.error)}
            />{:else}0{/if}
        </dd>
      </div>
    </dl>
  </div>
  <div class="task-group">
    <dl>
      {@render cell('Input tokens', integer(task.inputTokens))}
      {@render cell('Output tokens', integer(task.outputTokens))}
      {@render cell('Cache read', integer(task.cacheReadTokens))}
      {#if task.cacheWriteTokens}{@render cell(
          'Cache write',
          integer(task.cacheWriteTokens)
        )}{/if}
      {@render cell(
        'Cost (est., CAD)',
        cost(task.apiCostCad, task.estimateCoverage)
      )}
    </dl>
  </div>
</article>

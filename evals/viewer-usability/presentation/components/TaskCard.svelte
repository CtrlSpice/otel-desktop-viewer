<script lang="ts">
  import Badge from './Badge.svelte'
  import Icon from './Icon.svelte'
  import { ChevronDownIcon } from '@hugeicons/core-free-icons'
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
  const tools = $derived(task.runResults.flatMap(run =>
    (run.telemetry?.tools ?? []).map(tool => ({ ...tool, run: run.run }))
  ))
  const executionErrors = $derived(task.runResults.filter(run =>
    run.status === 'error' || run.status === 'harness-error'
  ))
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

{#snippet activityCounts()}
  <span class="activity-title">
    <span class="chevron" aria-hidden="true"><Icon icon={ChevronDownIcon} size={16} /></span>
    Tool calls
  </span>
  <span class="activity-counts">
    {#if task.toolCalls !== null && task.toolCalls > 0}
      <Badge status="tool-count" label="Total: {integer(task.toolCalls)}" />
    {/if}
    {#if task.failedTools !== null && task.failedTools > 0}
      <Badge status="tool-error" label="Failed: {integer(task.failedTools)}" />
    {/if}
    {#if task.error > 0}
      <Badge status="execution-error" label="Run errors: {integer(task.error)}" />
    {/if}
  </span>
{/snippet}

{#snippet spanLink(spanID: string | null, href: string | null)}
  {#if spanID && href}
    <a class="tool-span-link" {href} target="_blank" rel="noreferrer" aria-label="View span {spanID}">{spanID}</a>
  {:else}
    <span class="tool-time">Span unavailable</span>
  {/if}
{/snippet}

{#snippet toolActivity()}
  {#if (task.toolCalls !== 0 && tools.length > 0) || task.error > 0}
    <details class="task-tool-metric" data-live-key="tool-activity-{task.task}">
      <summary class="activity-summary">{@render activityCounts()}</summary>
      <ul class="tool-call-list">
        {#each tools as tool}
          <li class="tool-call-row">
            <span class="tool-call-description">
              {task.runResults.length > 1 ? `Run ${tool.run} · ` : ''}
              {#if tool.startedAt}
                <time data-local-time datetime={tool.startedAt} title={tool.startedAt}>{tool.startedAt.slice(0, 10)} {tool.startedAt.slice(11, 19)} UTC</time>
              {:else}
                <span class="tool-time">Time unavailable</span>
              {/if}
              · {tool.name}{#if tool.error}{' · '}<span class="tool-error">Error</span>{/if}
            </span>
            {@render spanLink(tool.spanID, tool.href)}
          </li>
        {/each}
        {#each executionErrors as run}
          <li class="tool-call-row">
            <span class="tool-call-description">
              Run {run.run} · <Badge status={run.status === 'harness-error' ? 'harness-error' : 'execution-error'} label="Needs attention" />
            </span>
            {@render spanLink(run.telemetry?.spanID ?? null, run.telemetry?.span ?? null)}
          </li>
        {/each}
      </ul>
    </details>
  {:else}
    <div class="task-tool-metric inactive-metric">
      <div class="activity-summary">{@render activityCounts()}</div>
    </div>
  {/if}
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
    </dl>
    {#each task.runResults as run}
      {#if run.telemetry}
        <dl>
          <div class="task-trace-row">
            <dt>{repetitions === 1 && task.runResults.length === 1 ? 'Trace' : `Run ${run.run} trace`}</dt>
            <dd>
              <a
                class="task-trace-link"
                href={run.telemetry.span ?? run.telemetry.trace}
                target="_blank"
                rel="noreferrer"
                title={run.telemetry.traceID}
                aria-label="View trace {run.telemetry.traceID} for {taskName(task.task)}, run {run.run}"
              >{run.telemetry.traceID}</a>
            </dd>
          </div>
        </dl>
      {/if}
    {/each}
    {@render toolActivity()}
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

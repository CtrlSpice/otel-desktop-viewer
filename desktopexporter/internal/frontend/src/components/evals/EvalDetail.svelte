<script lang="ts">
  import type { EvalRun, EvalAttempt } from './model'
  import {
    assessmentSummary,
    percent,
    cost,
    estimatedCost,
    localTimestamp,
    traceHref,
  } from './model'
  import AssessmentBadge from './AssessmentBadge.svelte'
  let {
    run,
    attempt,
    tab,
    onTab,
  }: {
    run: EvalRun
    attempt?: EvalAttempt
    tab: string
    onTab: (tab: string) => void
  } = $props()
  const investigator = $derived(estimatedCost(run.attempts, 'investigatorCost'))
  const grader = $derived(estimatedCost(run.attempts, 'graderCost'))
  const startedAt = $derived(
    attempt?.startedAt ?? (!attempt ? run.startedAt : undefined)
  )
</script>

<header class="eval-detail-heading">
  <p class="eval-eyebrow">{attempt ? 'Attempt details' : 'Run details'}</p>
  <h2>{attempt?.name ?? run.name}</h2>
  <div class="eval-meta">
    <span>{attempt?.state ?? run.state}</span><span
      >{attempt ? `Repetition ${attempt.repetition}` : run.evaluation}</span
    >
  </div>
  {#if startedAt}<time class="eval-time" datetime={startedAt} title={startedAt}
      >{localTimestamp(startedAt)}</time
    >{/if}
</header>

{#if attempt}
  <div class="eval-detail-tabs" role="group" aria-label="Attempt detail view">
    {#each ['Overview', 'Assessments', 'Tool calls'] as name}
      <button
        class:active={tab === name}
        aria-pressed={tab === name}
        onclick={() => onTab(name)}>{name}</button
      >
    {/each}
  </div>
{/if}

<!-- svelte-ignore a11y_no_noninteractive_tabindex (The scrollable inspector needs keyboard access even when its content contains no controls.) -->
<section class="eval-detail-body" tabindex="0" aria-label="Evaluation details">
  {#if !attempt}
    <section>
      <h3>Configuration</h3>
      <dl class="eval-properties">
        {#each run.configuration as field}<div>
            <dt>{field.name}</dt>
            <dd>{field.value}</dd>
          </div>{/each}
        <div>
          <dt>Source</dt>
          <dd>{run.source}</dd>
        </div>
      </dl>
    </section>
    <section>
      <h3>Assessments</h3>
      {#each run.assessments as definition}
        {@const summary = assessmentSummary(run, definition)}
        <div class="eval-assessment-summary">
          <div class="eval-between">
            <strong>{definition.name}</strong><span class="eval-number"
              >{percent(summary.percent)}</span
            >
          </div>
          <p>
            {summary.scored} scored · {definition.aggregation === 'planned'
              ? `${summary.denominator} planned`
              : `${summary.denominator} applicable scored attempts`}
          </p>
          <p>{definition.scale}</p>
        </div>
      {/each}
    </section>
    <section>
      <h3>Grading for this run</h3>
      <dl class="eval-properties">
        <div>
          <dt>Assessed attempts</dt>
          <dd>
            {run.attempts.filter(item => item.assessments.length > 0).length} / {run
              .attempts.length}
          </dd>
        </div>
        <div>
          <dt>Assessments recorded</dt>
          <dd>
            {run.attempts.reduce(
              (total, item) => total + item.assessments.length,
              0
            )}
          </dd>
        </div>
        <div>
          <dt>Grading estimate</dt>
          <dd>
            {cost(grader.total)}{grader.incomplete && grader.total !== null
              ? ' · incomplete'
              : ''}
          </dd>
        </div>
        <div>
          <dt>Investigator estimate</dt>
          <dd>
            {cost(investigator.total)}{investigator.incomplete &&
            investigator.total !== null
              ? ' · incomplete'
              : ''}
          </dd>
        </div>
      </dl>
      <p class="eval-note">
        Grades are provisional. Costs are CAD estimates, not billed spend.
      </p>
    </section>
  {:else if tab === 'Overview'}
    {#if attempt.error}<div class="eval-run-error">
        <strong>Needs attention</strong>
        <p>{attempt.error}</p>
      </div>{/if}
    <section>
      <h3>Question</h3>
      <p class="eval-prose">{attempt.question}</p>
    </section>
    <section>
      <h3>Answer</h3>
      <p class="eval-prose">{attempt.answer ?? 'No answer recorded.'}</p>
    </section>
    <section>
      <h3>Accounting</h3>
      <dl class="eval-properties">
        <div>
          <dt>Duration</dt>
          <dd>
            {attempt.seconds === undefined ? 'Unknown' : `${attempt.seconds} s`}
          </dd>
        </div>
        <div>
          <dt>Investigator estimate</dt>
          <dd>{cost(attempt.investigatorCost)}</dd>
        </div>
        <div>
          <dt>Grading estimate</dt>
          <dd>{cost(attempt.graderCost)}</dd>
        </div>
      </dl>
      <p class="eval-note">CAD estimates · not billed spend</p>
    </section>
    <section>
      <h3>Telemetry</h3>
      {#if attempt.telemetry}
        <a class="eval-trace-link" href={traceHref(attempt.telemetry)}
          >{attempt.telemetry.traceID}</a
        >
      {:else}<p class="eval-note">
          No trace attached. Results and assessments can be inspected
          independently.
        </p>{/if}
    </section>
  {:else if tab === 'Assessments'}
    <p class="eval-note">Provisional assessments · evaluated independently</p>
    {#each run.assessments as definition}
      {@const assessment = attempt.assessments.find(
        item => item.id === definition.id
      )}
      <section class="eval-assessment-card">
        <div class="eval-between">
          <h3>{definition.name}</h3>
          <AssessmentBadge {assessment} maximum={definition.maximum} />
        </div>
        {#if assessment}
          <p class="eval-prose">{assessment.explanation}</p>
          <dl class="eval-properties">
            <div>
              <dt>Evaluator</dt>
              <dd>{assessment.evaluator}</dd>
            </div>
            <div>
              <dt>Rubric</dt>
              <dd>{assessment.rubric}</dd>
            </div>
            <div>
              <dt>Score</dt>
              <dd>{assessment.value ?? 'Unscored'} / {definition.maximum}</dd>
            </div>
          </dl>
          {#if assessment.telemetry}<a
              class="eval-trace-link"
              title={`Grader trace ${assessment.telemetry.traceID}`}
              aria-label={`Grader trace ${assessment.telemetry.traceID}, span ${assessment.telemetry.spanID ?? 'not specified'}`}
              href={traceHref(assessment.telemetry)}
              >{assessment.telemetry.spanID ?? assessment.telemetry.traceID}</a
            >{/if}
        {:else}<p class="eval-note">No assessment recorded.</p>{/if}
      </section>
    {/each}
    <section>
      <h3>Review & corrections</h3>
      <p class="eval-note">No human signoff or corrections recorded.</p>
    </section>
  {:else}
    {@const failed = attempt.tools.filter(
      tool => tool.state === 'Error'
    ).length}
    <div class="eval-meta">
      <span class="eval-total">Total {attempt.tools.length}</span
      >{#if failed}<span class="eval-failed">Failed {failed}</span
        >{/if}{#if attempt.error}<span class="eval-attention">Run errors 1</span
        >{/if}
    </div>
    {#if !attempt.tools.length}<p class="eval-note">
        No tool calls recorded.
      </p>{/if}
    {#each attempt.tools as tool, index}
      <details class="eval-tool">
        <summary
          ><span class="eval-note">{String(index + 1).padStart(2, '0')}</span
          ><strong>{tool.name}</strong><span
            class:eval-failed={tool.state === 'Error'}>{tool.state}</span
          ></summary
        >
        <p class="eval-prose">{tool.message}</p>
        {#if tool.telemetry}<a
            class="eval-trace-link"
            href={traceHref(tool.telemetry)}
            >{tool.telemetry.spanID ?? tool.telemetry.traceID}</a
          >{/if}
      </details>
    {/each}
    {#if attempt.error}<div class="eval-run-error">
        <strong>Needs attention</strong>
        <p>{attempt.error}</p>
      </div>{/if}
  {/if}
</section>

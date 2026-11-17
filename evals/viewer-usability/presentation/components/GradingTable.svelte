<script lang="ts">
  import HugeiconsIcon from './Icon.svelte'
  import { ChevronDownIcon } from '@hugeicons/core-free-icons'
  import Badge from './Badge.svelte'
  import CompletionBadge from './CompletionBadge.svelte'
  import {
    cost,
    integer,
    seconds,
    taskName,
    taskOrder,
    runResult,
    groupGradingOutputs,
    gradingScoreTotals,
    gradingStatus,
    type Presentation,
    type Grader,
  } from './model'

  let { data, grader }: { data: Presentation; grader: Grader } = $props()
  const outputs = $derived(
    data.grader.outputs.filter(output => output.graderModel === grader.model)
  )
  const knownTasks = $derived([
    ...new Set(
      data.models.flatMap(model => model.tasks.map(task => task.task))
    ),
  ])
  const order = $derived([
    ...taskOrder.filter(id => knownTasks.includes(id)),
    ...knownTasks.filter(id => !taskOrder.includes(id)),
  ])
  const groups = $derived(
    groupGradingOutputs(
      outputs,
      data.models.map(model => model.model),
      order,
      true
    )
  )
  const runs = $derived(
    Array.from(
      {
        length: Math.max(
          data.repetitions ?? 0,
          ...groups.flatMap(group =>
            group.questions.map(question => question.grades.length)
          )
        ),
      },
      (_, index) => index
    )
  )
</script>

{#if groups.length}
  <!-- svelte-ignore a11y_no_noninteractive_tabindex (keyboard users must be able to scroll this region) -->
  <div
    class="grading-output"
    data-grading-scroll-key="grading-table"
    role="region"
    aria-label="Grading outputs"
    tabindex="0"
  >
    <table
      class="table grading-model-table"
      style="--grading-run-count:{runs.length}"
    >
      <caption class="sr-only">Grading outputs by investigator model</caption>
      <colgroup>
        <col class="grading-question-column" />
        {#each runs as _}<col class="grading-run-column" />{/each}
        <col span="4" class="grading-number-column" />
      </colgroup>
      {#each groups as group}
        {@const investigator = data.models.find(
          model => model.model === group.model
        )}
        {@const bodyID =
          'grading-body-' +
          encodeURIComponent(`${grader.model}:${group.model}`)}
        {@const scores = gradingScoreTotals(group.questions, runs.length)}
        {@const graded = scores.reduce((sum, score) => sum + score.graded, 0)}
        {@const status = gradingStatus(
          graded,
          data.tasksExpected !== null && data.repetitions !== null
            ? data.tasksExpected * data.repetitions
            : null,
          Math.max(
            investigator?.result.attempts ?? 0,
            outputs.filter(output => output.investigatorModel === group.model)
              .length
          ),
          Math.max(
            investigator?.result.error ?? 0,
            scores.reduce((sum, score) => sum + score.errors, 0)
          )
        )}
        <tbody class="grading-model-header">
          <tr>
            <th scope="col">
              <button
                type="button"
                class="grading-table-toggle"
                id="toggle-{bodyID}"
                data-grading-table={bodyID}
                aria-controls={bodyID}
                aria-expanded="false"
              >
                <span class="grading-table-caret" aria-hidden="true"
                  ><HugeiconsIcon
                    icon={ChevronDownIcon}
                    size={16}
                    strokeWidth={1.5}
                  /></span
                >
                <span class="grading-model-name"
                  >{investigator?.name ?? group.model}</span
                >
                <CompletionBadge {status} />
              </button>
            </th>
            {#each runs as index}<th scope="col" class="grading-run"
                >Score {index + 1}</th
              >{/each}
            <th scope="col">Grading Time</th>
            <th scope="col">Cost (est., CAD)</th>
            <th scope="col">Input Tokens</th>
            <th scope="col">Output Tokens</th>
          </tr>
        </tbody>
        <tbody class="grading-questions" id={bodyID} hidden>
          {#each group.questions as question}
            <tr>
              <th scope="row">{taskName(question.task)}</th>
              {#each runs as index}
                {@const output = question.grades[index]}
                <td class="grading-run">
                  {#if !output}<span aria-label="Not recorded">—</span>
                  {:else if output.executionError}<Badge
                      status="execution-error"
                      label="Execution error"
                    />
                  {:else if output.score === null}<Badge
                      status="unscored"
                      label="Unscored"
                    />
                  {:else}<Badge
                      status={output.score === 1
                        ? 'complete'
                        : output.score === 0.5
                          ? 'partial'
                          : 'no-credit'}
                      label={runResult(output.score)}
                    />{/if}{#if output?.telemetry?.span}
                    <a class="grader-trace-link" href={output.telemetry.span} target="_blank" rel="noreferrer" aria-label="{taskName(question.task)} run {index + 1} grader trace">Trace</a>
                  {/if}
                </td>
              {/each}
              <td>{seconds(question.totalSeconds)}</td>
              <td>{cost(question.costCad, question.estimateCoverage)}</td>
              <td>{integer(question.inputTokens)}</td>
              <td>{integer(question.outputTokens)}</td>
            </tr>
          {/each}
        </tbody>
        <tbody class="grading-model-total">
          <tr>
            <th scope="row">Total</th>
            {#each scores as score}
              <td class="grading-run"
                ><span class="grading-score-total"
                  >{score.earned === 0.5
                    ? '.5'
                    : String(score.earned ?? 0)}/{integer(
                    data.tasksExpected
                  )}</span
                ></td
              >
            {/each}
            <td>{seconds(group.totals.totalSeconds)}</td>
            <td>{cost(group.totals.costCad, group.totals.estimateCoverage)}</td>
            <td>{integer(group.totals.inputTokens)}</td>
            <td>{integer(group.totals.outputTokens)}</td>
          </tr>
        </tbody>
      {/each}
    </table>
  </div>
{/if}

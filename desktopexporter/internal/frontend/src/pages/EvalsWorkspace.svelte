<script lang="ts">
  import PageLayout from '@/components/shared/PageLayout.svelte'
  import SignalCard from '@/components/shared/SignalCard.svelte'
  import SignalFooter from '@/components/shared/SignalFooter.svelte'
  import EvalDetail from '@/components/evals/EvalDetail.svelte'
  import AssessmentBadge from '@/components/evals/AssessmentBadge.svelte'
  import { exampleRuns } from '@/components/evals/examples'
  import {
    assessmentSummary,
    localTimestamp,
    percent,
  } from '@/components/evals/model'
  import { evalsEnabled } from '@/state/features'
  import { getRouteContext } from '@/contexts/route-context.svelte'
  import { navigate, buildSearch, withQueryPatch } from '@/route'
  import type { ImportFailure } from '@/types/import-types'
  import '@/components/evals/evals.css'

  let {
    importFailure = null,
    onViewIssue,
  }: {
    importFailure?: ImportFailure | null
    onViewIssue?: (event: MouseEvent) => void
  } = $props()
  const route = getRouteContext()
  let runSearch = $state('')
  const query = $derived(route.route.query)
  const run = $derived(
    exampleRuns.find(item => item.id === query.run) ?? exampleRuns[0]
  )
  const attempt = $derived(run.attempts.find(item => item.id === query.attempt))
  const tab = $derived(
    ['Overview', 'Assessments', 'Tool calls'].includes(query.detail ?? '')
      ? (query.detail ?? 'Overview')
      : 'Overview'
  )
  const runs = $derived(
    exampleRuns.filter(item =>
      `${item.name} ${item.evaluation} ${item.state}`
        .toLowerCase()
        .includes(runSearch.toLowerCase())
    )
  )
  const results = $derived.by(() => {
    const text = (query.q ?? '').toLowerCase().trim()
    const rows = run.attempts.filter(item => {
      if (query.status && item.state !== query.status) return false
      const searchable = [
        item.name,
        item.question,
        item.answer ?? '',
        item.state,
        ...item.assessments.flatMap(score => [
          score.id,
          run.assessments.find(def => def.id === score.id)?.name ?? '',
          score.label,
          score.explanation,
          score.evaluator,
        ]),
      ]
        .join(' ')
        .toLowerCase()
      return !text || searchable.includes(text)
    })
    if (query.sort === 'duration')
      rows.sort((a, b) => (b.seconds ?? -1) - (a.seconds ?? -1))
    if (query.sort === 'name') rows.sort((a, b) => a.name.localeCompare(b.name))
    return rows
  })
  const selectedIndex = $derived(
    results.findIndex(item => item.id === attempt?.id)
  )
  const completed = $derived(
    run.attempts.filter(item => item.state === 'Complete').length
  )

  function update(changes: Record<string, string | null>, replace = false) {
    navigate(
      `/evals${buildSearch(withQueryPatch(query, changes))}`,
      replace ? 'replace' : 'push'
    )
  }
  function selectRun(id: string) {
    update({ run: id, attempt: null })
  }
  function selectIndex(index: number) {
    const item = results[index]
    if (item) update({ run: run.id, attempt: item.id })
  }
</script>

<div class="evals-page">
  <PageLayout
    items={$evalsEnabled ? runs : []}
    selectedID={run.id}
    drawerID="signal-drawer"
    drawerLabel="Evals"
    itemKey={item => item.id}
    defaultDetailRem={25}
    minMainWidth={0.35}
    minDetailWidth={0.25}
    resizableStorageKey="evals:detail-width"
  >
    {#snippet drawerSearch()}
      <label class="eval-run-search"
        ><span class="sr-only">Find a run</span><input
          type="search"
          placeholder="Find a run…"
          bind:value={runSearch}
        /></label
      >
    {/snippet}
    {#snippet drawerChromeToolbar()}<span class="eval-drawer-label"
        >Evaluation runs</span
      >{/snippet}
    {#snippet itemSnippet(item, selected)}
      <SignalCard
        id={item.id}
        title={item.name}
        description={item.evaluation}
        {selected}
        onclick={selectRun}
      >
        {#snippet badge()}<span
            class="eval-run-state"
            class:eval-attention={item.state === 'Paused'}>{item.state}</span
          >{/snippet}
        {#snippet meta()}
          <div class="eval-run-card-meta">
            <div class="eval-run-meta">
              <span
                >{item.attempts.filter(row => row.state === 'Complete').length} /
                {item.attempts.length} complete</span
              >
              <strong
                >{percent(
                  assessmentSummary(
                    item,
                    item.assessments.find(
                      definition => definition.id === item.headlineAssessment
                    ) ?? item.assessments[0]
                  ).percent
                )}</strong
              >
            </div>
            <div class="eval-progress" aria-hidden="true">
              <span
                style:width={`${(item.attempts.filter(row => row.state === 'Complete').length / item.attempts.length) * 100}%`}
              ></span>
            </div>
            {#if item.startedAt}<time
                class="eval-time"
                datetime={item.startedAt}
                title={item.startedAt}>{localTimestamp(item.startedAt)}</time
              >{/if}
          </div>
        {/snippet}
      </SignalCard>
    {/snippet}
    {#snippet drawerFooter()}<div class="eval-drawer-footer">
        {runs.length} runs <span>Example data</span>
      </div>{/snippet}
    {#snippet main()}
      {#if !$evalsEnabled}
        <div class="eval-empty">
          <h1>Evals is not enabled</h1>
          <p>
            Enable Evals in your local feature flags to open this experiment.
          </p>
        </div>
      {:else}
        {#if importFailure}<div class="alert alert-error">
            Import issue in {importFailure.fileName}.
            <a href="/#ingestion-issues" onclick={onViewIssue}>View issue</a>
          </div>{/if}
        <header class="eval-run-heading">
          <div class="eval-between">
            <p class="eval-eyebrow">{run.evaluation}</p>
            <span class="eval-example">Example data</span>
          </div>
          <div class="eval-between">
            <h1>{run.name}</h1>
            <button
              class="eval-text-button"
              onclick={() => update({ attempt: null })}>Run details</button
            >
          </div>
          <div class="eval-meta">
            <span>{run.state}</span><span
              >{completed} / {run.attempts.length} complete</span
            ><span>Grades provisional</span>
          </div>
          <div class="eval-summary-grid">
            {#each run.assessments as definition}
              {@const summary = assessmentSummary(run, definition)}
              <div class="eval-metric">
                <span>{definition.name}</span><strong
                  >{percent(summary.percent)}</strong
                >
                <div class="eval-progress" aria-hidden="true">
                  <span style:width={`${summary.percent ?? 0}%`}></span>
                </div>
                <small
                  >{summary.scored} scored / {summary.denominator}
                  {definition.aggregation === 'planned'
                    ? 'planned'
                    : 'scored applicable'}</small
                >
              </div>
            {/each}
          </div>
        </header>
        <div class="eval-result-toolbar">
          <label class="eval-result-search"
            ><span class="sr-only">Filter loaded results</span><input
              type="search"
              placeholder="Filter loaded results…"
              value={query.q ?? ''}
              oninput={event => update({ q: event.currentTarget.value }, true)}
            /></label
          >
          <select
            aria-label="Execution status"
            value={query.status ?? ''}
            onchange={event =>
              update({ status: event.currentTarget.value }, true)}
            ><option value="">All statuses</option
            >{#each ['Complete', 'Running', 'Error', 'Planned'] as state}<option
                >{state}</option
              >{/each}</select
          >
          <select
            aria-label="Sort results"
            value={query.sort ?? ''}
            onchange={event =>
              update({ sort: event.currentTarget.value }, true)}
            ><option value="">Case order</option><option value="duration"
              >Slowest first</option
            ><option value="name">Name</option></select
          >
        </div>
        <div class="eval-results-scroll">
          <table class="eval-results">
            <caption class="sr-only">Results for {run.name}</caption>
            <thead
              ><tr
                ><th scope="col">Case / attempt</th
                >{#each run.assessments as definition}<th
                    scope="col"
                    title={definition.scale}>{definition.name}</th
                  >{/each}</tr
              ></thead
            >
            <tbody>
              {#each results as item (item.id)}
                <tr class:eval-selected={attempt?.id === item.id}>
                  <th scope="row"
                    ><button
                      aria-pressed={attempt?.id === item.id}
                      onclick={() => update({ run: run.id, attempt: item.id })}
                      ><strong>{item.name}</strong><span class="eval-meta"
                        ><span class:eval-attention={item.state === 'Error'}
                          >{item.state}</span
                        >{#if item.seconds !== undefined}<span
                            >{item.seconds} s</span
                          >{/if}<span>Rep {item.repetition}</span></span
                      ></button
                    ></th
                  >
                  {#each run.assessments as definition}<td
                      ><AssessmentBadge
                        assessment={item.assessments.find(
                          score => score.id === definition.id
                        )}
                        maximum={definition.maximum}
                      /></td
                    >{/each}
                </tr>
              {/each}
            </tbody>
          </table>
          {#if !results.length}<div class="eval-empty">
              <h2>No matching attempts</h2>
              <p>Try another term or execution status.</p>
              <button
                class="eval-text-button"
                onclick={() => update({ q: null, status: null }, true)}
                >Clear filters</button
              >
            </div>{/if}
        </div>
        <div class="eval-results-note">
          <span>{results.length} / {run.attempts.length} attempts</span><span
            >— No assessment recorded</span
          >
        </div>
      {/if}
    {/snippet}
    {#snippet detail()}
      {#if $evalsEnabled}
        {#key `${run.id}/${attempt?.id ?? ''}`}<EvalDetail
            {run}
            {attempt}
            {tab}
            onTab={detail => update({ detail }, true)}
          />{/key}
      {:else}<div class="eval-empty">
          Select an enabled evaluation to inspect its results.
        </div>{/if}
    {/snippet}
    {#snippet pageFooter()}
      <div class="eval-footer">
        <span>Experimental and unsupported</span>
        {#if $evalsEnabled}<SignalFooter
            index={selectedIndex}
            total={results.length}
            label="attempt"
            onFirst={() => selectIndex(0)}
            onPrev={() => selectIndex(selectedIndex - 1)}
            onNext={() => selectIndex(selectedIndex + 1)}
            onLast={() => selectIndex(results.length - 1)}
          />{/if}
      </div>
    {/snippet}
  </PageLayout>
</div>

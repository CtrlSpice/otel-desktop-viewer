<script lang="ts">
  import PageLayout from '@/components/shared/PageLayout.svelte'
  import PaneHeader from '@/components/shared/PaneHeader.svelte'
  import EvalRunSummary from '@/components/evals/EvalRunSummary.svelte'
  import RunStatusBadge from '@/components/evals/RunStatusBadge.svelte'
  import SignalFooter from '@/components/shared/SignalFooter.svelte'
  import SignalDrawerFooter from '@/components/shared/Drawer/SignalDrawerFooter.svelte'
  import EvalDetail from '@/components/evals/EvalDetail.svelte'
  import AssessmentBadge from '@/components/evals/AssessmentBadge.svelte'
  import { exampleRuns } from '@/components/evals/examples'
  import { getRouteContext } from '@/contexts/route-context.svelte'
  import { navigate, buildSearch, withQueryPatch } from '@/route'
  import { INGESTION_ISSUES_ID } from '@/components/shared/IngestionIssues.svelte'
  import type { ImportFailure } from '@/types/import-types'
  import { PANEL_DEFAULT_REM } from '@/state/panel-width'

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

<PageLayout
  items={runs}
  selectedID={run.id}
  drawerID="signal-drawer"
  drawerLabel="Evals"
  itemKey={item => item.id}
  defaultDetailRem={PANEL_DEFAULT_REM}
  resizableStorageKey="evals:detail-width"
>
  {#snippet drawerSearch()}
    <input
      class="input typed-field typed-field--pill w-full"
      type="search"
      aria-label="Find a run"
      placeholder="Find a run…"
      bind:value={runSearch}
    />
  {/snippet}
  {#snippet drawerChromeToolbar()}{/snippet}
  {#snippet itemSnippet(item, selected)}
    <EvalRunSummary run={item} {selected} onclick={selectRun} />
  {/snippet}
  {#snippet drawerFooter()}<SignalDrawerFooter
      count={runs.length}
      label="run"
    />{/snippet}
  {#snippet main()}
    {#if importFailure}<div class="alert alert-error">
        Import issue in {importFailure.fileName}.
        <a class="link" href={`/#${INGESTION_ISSUES_ID}`} onclick={onViewIssue}
          >View issue</a
        >
      </div>{/if}
    <PaneHeader mode="title" title={run.name} subtitle={run.evaluation}>
      {#snippet badge()}<RunStatusBadge state={run.state} />{/snippet}
      {#snippet right()}<button
          class="btn btn-ghost btn-xs"
          onclick={() => update({ attempt: null })}>Run details</button
        >{/snippet}
    </PaneHeader>
    <PaneHeader mode="toolbar" ariaLabel="Result filters" rounded={false}>
      {#snippet right()}
        <div class="flex w-full min-w-0 flex-wrap items-center gap-2">
          <input
            class="input typed-field typed-field--pill min-w-0 flex-1"
            type="search"
            aria-label="Filter loaded results"
            placeholder="Filter loaded results…"
            value={query.q ?? ''}
            oninput={event => update({ q: event.currentTarget.value }, true)}
          />
          <select
            class="select typed-field w-auto"
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
            class="select typed-field w-auto"
            aria-label="Sort results"
            value={query.sort ?? ''}
            onchange={event =>
              update({ sort: event.currentTarget.value }, true)}
            ><option value="">Case order</option><option value="duration"
              >Slowest first</option
            ><option value="name">Name</option></select
          >
        </div>
      {/snippet}
    </PaneHeader>
    <div class="min-h-0 flex-1 overflow-auto">
      <table class="table w-full">
        <caption class="sr-only">Results for {run.name}</caption>
        <thead class="header-surface sticky top-0 z-10"
          ><tr
            ><th class="table-header-cell text-left" scope="col"
              >Case / attempt</th
            >{#each run.assessments as definition}<th
                class="table-header-cell text-right"
                scope="col"
                title={definition.scale}>{definition.name}</th
              >{/each}</tr
          ></thead
        >
        <tbody class="table-body-surface">
          {#each results as item (item.id)}
            <tr
              class="table-row"
              class:table-row--selected={attempt?.id === item.id}
            >
              <th scope="row" class="table-cell text-left font-normal"
                ><button
                  class="btn btn-ghost btn-sm h-auto justify-start text-left whitespace-normal"
                  aria-pressed={attempt?.id === item.id}
                  onclick={() => update({ run: run.id, attempt: item.id })}
                  >{item.name}</button
                >
                <div class="flex flex-wrap items-center gap-2 px-3 text-xs">
                  <span
                    class="badge badge-xs badge-soft"
                    class:badge-error={item.state === 'Error'}
                    >{item.state}</span
                  >{#if item.seconds !== undefined}<span class="tabular-nums"
                      >{item.seconds} s</span
                    >{/if}<span>Rep {item.repetition}</span>
                </div></th
              >
              {#each run.assessments as definition}<td
                  class="table-cell text-right"
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
      {#if !results.length}<div class="p-6 text-sm">
          <p>No matching attempts</p>
          <button
            class="btn btn-ghost btn-sm"
            onclick={() => update({ q: null, status: null }, true)}
            >Clear filters</button
          >
        </div>{/if}
    </div>
  {/snippet}
  {#snippet detail()}
    {#key `${run.id}/${attempt?.id ?? ''}`}<EvalDetail
        {run}
        {attempt}
        {tab}
        onTab={detail => update({ detail }, true)}
      />{/key}
  {/snippet}
  {#snippet pageFooter()}
    <div class="flex min-w-0 flex-wrap items-center">
      <span class="px-3 text-xs">Experimental and unsupported</span>
      <div class="min-w-0 flex-1">
        <SignalFooter
          index={selectedIndex}
          total={results.length}
          label="attempt"
          onFirst={() => selectIndex(0)}
          onPrev={() => selectIndex(selectedIndex - 1)}
          onNext={() => selectIndex(selectedIndex + 1)}
          onLast={() => selectIndex(results.length - 1)}
        />
      </div>
    </div>
  {/snippet}
</PageLayout>

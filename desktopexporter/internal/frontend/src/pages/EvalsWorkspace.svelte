<script lang="ts">
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { Cancel01Icon } from '@hugeicons/core-free-icons'
  import { QuestionMarkIcon } from '@/icons'
  import PageLayout from '@/components/shared/PageLayout.svelte'
  import PaneHeader from '@/components/shared/PaneHeader.svelte'
  import DateTimeFilter from '@/components/shared/Toolbar/DateTimeFilter.svelte'
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

  let {
    importFailure = null,
    onViewIssue,
  }: {
    importFailure?: ImportFailure | null
    onViewIssue?: (event: MouseEvent) => void
  } = $props()
  const route = getRouteContext()
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
  const results = $derived.by(() => {
    const text = (query.q ?? '').toLowerCase().trim()
    return run.attempts.filter(item => {
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
  items={exampleRuns}
  selectedID={run.id}
  drawerID="signal-drawer"
  drawerLabel="Evals"
  itemKey={item => item.id}
>
  {#snippet drawerSearch()}
    <div class="flex w-full min-w-0 items-center gap-1">
      <input
        class="input typed-field typed-field--pill min-w-0 flex-1"
        type="search"
        aria-label="Search evals"
        placeholder="Search evals…"
        value={query.q ?? ''}
        oninput={event => update({ q: event.currentTarget.value }, true)}
      />
      <button
        type="button"
        class="btn btn-circle btn-ghost btn-sm shrink-0 text-sm text-base-content"
        aria-label="Clear search"
        onclick={() => update({ q: null }, true)}
      >
        <HugeiconsIcon
          icon={Cancel01Icon}
          size={17}
          strokeWidth={1.5}
          aria-hidden="true"
        />
      </button>
      <button
        type="button"
        class="btn btn-circle btn-ghost btn-sm shrink-0 text-sm text-base-content"
        aria-label="Search query help"
      >
        <QuestionMarkIcon
          class="h-[17px] w-[17px] shrink-0"
          aria-hidden="true"
        />
      </button>
    </div>
  {/snippet}
  {#snippet drawerChromeToolbar()}<DateTimeFilter />{/snippet}
  {#snippet itemSnippet(item, selected)}
    <EvalRunSummary run={item} {selected} onclick={selectRun} />
  {/snippet}
  {#snippet drawerFooter()}<SignalDrawerFooter
      count={exampleRuns.length}
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
      {#snippet badge()}<RunStatusBadge
          state={run.state}
          pauseReason={run.pauseReason}
        />{/snippet}
      {#snippet right()}<button
          class="btn btn-ghost btn-xs"
          onclick={() => update({ attempt: null })}>Run details</button
        >{/snippet}
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
                  class="btn btn-ghost btn-sm h-auto justify-start text-left whitespace-normal hover:bg-transparent"
                  aria-pressed={attempt?.id === item.id}
                  onclick={() => update({ run: run.id, attempt: item.id })}
                  >{item.name}</button
                >
                <div class="flex flex-wrap items-center gap-2 px-3 text-xs">
                  <span
                    class="badge badge-xs badge-soft"
                    class:badge-papaya={item.state === 'Error'}
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
            onclick={() => update({ q: null }, true)}>Clear search</button
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

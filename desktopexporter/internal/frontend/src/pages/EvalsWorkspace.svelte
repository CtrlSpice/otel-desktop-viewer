<script lang="ts">
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { Cancel01Icon } from '@hugeicons/core-free-icons'
  import { QuestionMarkIcon } from '@/icons'
  import PageLayout from '@/components/shared/PageLayout.svelte'
  import DateTimeFilter from '@/components/shared/Toolbar/DateTimeFilter.svelte'
  import EvalRunSummary from '@/components/evals/EvalRunSummary.svelte'
  import EvalResults from '@/components/evals/EvalResults.svelte'
  import SignalFooter from '@/components/shared/SignalFooter.svelte'
  import EvalDetail from '@/components/evals/EvalDetail.svelte'
  import TraceDetailView from '@/components/traces/Detail/TraceDetailView.svelte'
  import PaneHeader from '@/components/shared/PaneHeader.svelte'
  import ExportButton from '@/components/shared/ExportButton.svelte'
  import type { SpanNode } from '@/types/api-types'
  import { referenceKey } from '@/components/evals/eval-traces'
  import { exampleRuns } from '@/components/evals/examples'
  import { getRouteContext } from '@/contexts/route-context.svelte'
  import {
    navigate,
    navigateToItem,
    buildSearch,
    withQueryPatch,
    SPAN_PARAM,
  } from '@/route'
  import { INGESTION_ISSUES_ID } from '@/components/shared/IngestionIssues.svelte'
  import type { ImportFailure } from '@/types/import-types'
  import { detailPanel } from '@/state/detail-panel.svelte'

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
  let selectedSpan = $state<SpanNode>()
  $effect(() => {
    void run.id
    void attempt?.id
    selectedSpan = undefined
  })
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
    selectedSpan = undefined
    detailPanel.reveal()
    update({ run: id, attempt: null })
  }
  function showDetail(attempt: string | null) {
    selectedSpan = undefined
    detailPanel.reveal()
    update({ run: run.id, attempt })
  }
  function selectIndex(index: number) {
    const item = results[index]
    if (item) showDetail(item.id)
  }
</script>

<PageLayout
  mainSurface="base"
  items={exampleRuns}
  navigationCount={exampleRuns.length}
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
  {#snippet main()}
    {#if importFailure}<div class="alert alert-error">
        Import issue in {importFailure.fileName}.
        <a class="link" href={`/#${INGESTION_ISSUES_ID}`} onclick={onViewIssue}
          >View issue</a
        >
      </div>{/if}
    <EvalResults
      {run}
      attempts={results}
      selectedID={attempt?.id}
      onSelect={showDetail}
      selectedSpanKey={selectedSpan
        ? referenceKey(selectedSpan.spanData)
        : undefined}
      onSelectSpan={node => {
        selectedSpan = node
        detailPanel.reveal()
      }}
    />
    {#if !results.length}<div class="p-6 text-sm">
        <p>No matching attempts</p>
        <button
          class="btn btn-ghost btn-sm"
          onclick={() => update({ q: null }, true)}>Clear search</button
        >
      </div>{/if}
  {/snippet}
  {#snippet detail()}
    {#if selectedSpan}
      {@const node = selectedSpan}
      <PaneHeader pill mode="title" title={selectedSpan.spanData.name}>
        {#snippet right()}
          <button
            class="btn btn-ghost btn-sm"
            onclick={() => (selectedSpan = undefined)}>Back to eval</button
          >
          <ExportButton signal="trace" id={node.spanData.traceID} />
        {/snippet}
      </PaneHeader>
      {#key referenceKey(selectedSpan.spanData)}
        <TraceDetailView
          span={selectedSpan.spanData}
          salvaged={selectedSpan.salvaged}
          cyclePoint={selectedSpan.cyclePoint}
          onSelectParent={spanID =>
            navigateToItem('traces', node.spanData.traceID, 'push', {
              [SPAN_PARAM]: spanID,
            })}
        />
      {/key}
    {:else}
      {#key `${run.id}/${attempt?.id ?? ''}`}<EvalDetail
          {run}
          {attempt}
        />{/key}
    {/if}
  {/snippet}
  {#snippet pageFooter()}
    <div class="flex min-w-0 flex-wrap items-center">
      <span class="px-3 text-sm text-rp-subtle"
        >Experimental and unsupported</span
      >
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

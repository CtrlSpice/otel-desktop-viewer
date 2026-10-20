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
  import EvalRowDetail from '@/components/evals/EvalRowDetail.svelte'
  import EvalSnapshotLibrary from '@/components/evals/EvalSnapshotLibrary.svelte'
  import { projectEvalSnapshot } from '@/components/evals/snapshot-adapters'
  import {
    telemetryAPI,
    isAbortError,
    type EvalSnapshot,
  } from '@/services/telemetry-service'
  import {
    buildEvalOutline,
    type EvalOutlineRow,
  } from '@/components/evals/eval-outline'
  import { groupAttempts } from '@/components/evals/group-attempts'
  import { indexEvalTraces } from '@/components/evals/eval-traces'
  import { exampleRuns } from '@/components/evals/examples'
  import { getRouteContext } from '@/contexts/route-context.svelte'
  import { navigate, buildSearch, withQueryPatch } from '@/route'
  import { INGESTION_ISSUES_ID } from '@/components/shared/IngestionIssues.svelte'
  import type { ImportFailure } from '@/types/import-types'
  import { detailPanel } from '@/state/detail-panel.svelte'
  import type { TraceData } from '@/types/api-types'

  let {
    importFailure = null,
    onViewIssue,
  }: {
    importFailure?: ImportFailure | null
    onViewIssue?: (event: MouseEvent) => void
  } = $props()
  const route = getRouteContext()
  const query = $derived(route.route.query)
  const snapshotID = $derived(query.snapshot)
  let snapshot = $state<EvalSnapshot>()
  let snapshotLoading = $state(false)
  let snapshotError = $state('')
  const currentSnapshot = $derived(
    snapshot?.id === query.snapshot ? snapshot : undefined
  )
  const projection = $derived(
    currentSnapshot
      ? projectEvalSnapshot({
          id: currentSnapshot.id,
          name: currentSnapshot.name,
          source: currentSnapshot.source,
          format: currentSnapshot.format,
          payload: currentSnapshot.originalJSON,
        })
      : undefined
  )
  const runs = $derived(
    query.snapshot
      ? projection?.status === 'supported'
        ? projection.runs
        : []
      : exampleRuns
  )
  const run = $derived(runs.find(item => item.id === query.run) ?? runs[0])
  const attempt = $derived(
    run?.attempts.find(item => item.id === query.attempt)
  )
  let resolvedRow = $state<EvalOutlineRow>()
  let traceViews = $state<TraceData[]>([])
  let telemetryLoading = $state(false)
  const telemetryNodes = $derived(indexEvalTraces(traceViews).nodes)
  let evalView = $state('results')
  const detailRowID = $derived(
    query.row ??
      (evalView === 'time' && attempt ? `attempt/${attempt.id}` : undefined)
  )
  const fallbackOutline = $derived(
    run
      ? buildEvalOutline(
          groupAttempts(run.attempts, run.attempts),
          run,
          indexEvalTraces([])
        )
      : []
  )
  const selectedRow = $derived(
    detailRowID && resolvedRow?.id === detailRowID
      ? resolvedRow
      : detailRowID
        ? fallbackOutline.find(row => row.id === detailRowID)
        : undefined
  )
  const parentName = $derived(
    fallbackOutline.find(row => row.id === selectedRow?.parentID)?.name
  )
  const results = $derived.by(() => {
    if (!run) return []
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
  const sourcePointer = $derived(
    projection?.status === 'supported'
      ? projection.rows.find(
          row => row.attemptID === (selectedRow?.attempt?.id ?? attempt?.id)
        )?.pointer
      : undefined
  )

  $effect(() => {
    const id = snapshotID
    snapshot = undefined
    snapshotError = ''
    snapshotLoading = !!id
    if (!id) return
    const controller = new AbortController()
    telemetryAPI
      .getEvalSnapshot(id, controller.signal)
      .then(value => {
        if (!controller.signal.aborted) snapshot = value
      })
      .catch(error => {
        if (!controller.signal.aborted && !isAbortError(error))
          snapshotError =
            error instanceof Error ? error.message : 'Could not open snapshot.'
      })
      .finally(() => {
        if (!controller.signal.aborted) snapshotLoading = false
      })
    return () => controller.abort()
  })

  $effect(() => {
    void run?.id
    resolvedRow = undefined
    traceViews = []
    telemetryLoading = false
  })

  // A snapshot locator without a run opens its first projection; store the
  // projected identity so reloading or sharing the route preserves selection.
  $effect(() => {
    if (query.snapshot && run && !query.run) update({ run: run.id }, true)
  })

  function update(changes: Record<string, string | null>, replace = false) {
    navigate(
      `/evals${buildSearch(withQueryPatch(query, changes))}`,
      replace ? 'replace' : 'push'
    )
  }
  function selectRun(id: string) {
    detailPanel.reveal()
    update({ run: id, attempt: null, row: null })
  }
  function showDetail(attempt: string | null) {
    if (!run) return
    detailPanel.reveal()
    update({ run: run.id, attempt, row: null })
  }
  function selectRow(row: EvalOutlineRow) {
    if (!run) return
    detailPanel.reveal()
    update({
      run: run.id,
      attempt: row.attempt?.id ?? null,
      row: row.kind === 'attempt' ? null : row.id,
    })
  }
  function selectIndex(index: number) {
    const item = results[index]
    if (item) showDetail(item.id)
  }
  function showSynthetic() {
    update({ snapshot: null, run: 'direct', attempt: null, row: null, q: null })
  }
</script>

<PageLayout
  mainSurface="base"
  items={runs}
  navigationCount={runs.length}
  selectedID={run?.id ?? null}
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
    <EvalSnapshotLibrary
      snapshot={currentSnapshot}
      {projection}
      snapshotID={query.snapshot}
      loading={snapshotLoading}
      error={snapshotError}
      {sourcePointer}
      onOpen={id =>
        update({ snapshot: id, run: null, attempt: null, row: null, q: null })}
      onDelete={id => {
        if (query.snapshot === id) showSynthetic()
      }}
      onSynthetic={showSynthetic}
    />
    {#if importFailure}<div class="alert alert-error">
        Import issue in {importFailure.fileName}.
        <a class="link" href={`/#${INGESTION_ISSUES_ID}`} onclick={onViewIssue}
          >View issue</a
        >
      </div>{/if}
    {#if run}
      {#key `${query.snapshot ?? 'synthetic'}/${run.id}`}
        <EvalResults
          bind:viewKey={evalView}
          {run}
          attempts={results}
          selectedID={attempt?.id}
          selectedRowID={query.row}
          onSelect={selectRow}
          onSelectRun={() => showDetail(null)}
          onResolveSelection={row => (resolvedRow = row)}
          onTraceViews={(traces, loading) => {
            traceViews = traces
            telemetryLoading = loading
          }}
        />
      {/key}
      {#if !results.length}<div class="p-6 text-sm">
          <p>No matching attempts</p>
          <button
            class="btn btn-ghost btn-sm"
            onclick={() => update({ q: null }, true)}>Clear search</button
          >
        </div>{/if}
    {:else if projection?.status === 'supported'}
      <p class="p-3 text-sm text-rp-subtle">
        This snapshot has no projected runs. Inspect its original JSON.
      </p>
    {/if}
  {/snippet}
  {#snippet detail()}
    {#if run}
      {#key `${run.id}/${attempt?.id ?? ''}/${selectedRow?.id ?? ''}/${evalView}`}
        {#if selectedRow}<EvalRowDetail
            {run}
            row={selectedRow}
            traceLevel={evalView === 'time'}
            {parentName}
            {telemetryNodes}
            {telemetryLoading}
          />{:else}<EvalDetail
            {run}
            {attempt}
            {telemetryNodes}
            {telemetryLoading}
          />{/if}
      {/key}
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

<script lang="ts">
  import { onMount } from 'svelte'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { Cancel01Icon } from '@hugeicons/core-free-icons'
  import { QuestionMarkIcon } from '@/icons'
  import PageLayout from '@/components/shared/PageLayout.svelte'
  import PaneHeader from '@/components/shared/PaneHeader.svelte'
  import DateTimeFilter from '@/components/shared/Toolbar/DateTimeFilter.svelte'
  import DrawerSortControl from '@/components/shared/Drawer/DrawerSortControl.svelte'
  import ListHeaderControls from '@/components/shared/ListHeaderControls.svelte'
  import {
    compareEvalRuns,
    type EvalRunSort,
  } from '@/components/evals/run-sort'
  import EvalRunSummary from '@/components/evals/EvalRunSummary.svelte'
  import EvalOutcomeFilter from '@/components/evals/EvalOutcomeFilter.svelte'
  import { toRunCard } from '@/components/evals/run-summary'
  import {
    outcomeKey,
    readRunOutcomeFilter,
    runOutcomeOptions,
  } from '@/components/evals/run-outcome-filter'
  import EvalResults from '@/components/evals/EvalResults.svelte'
  import SignalFooter from '@/components/shared/SignalFooter.svelte'
  import DetailNav from '@/components/shared/DetailNav.svelte'
  import EvalDetail from '@/components/evals/EvalDetail.svelte'
  import EvalRowDetail from '@/components/evals/EvalRowDetail.svelte'
  import {
    projectPromptfooDataset,
    evalProjectionLimits,
    type EvalSourceDataset,
    type EvalSourceProjection,
  } from '@/components/evals/promptfoo-adapter'
  import {
    evalSourceFieldCopy,
    evalSourceRunCopy,
  } from '@/components/evals/eval-source-copy'
  import { telemetryAPI, isAbortError } from '@/services/telemetry-service'
  import {
    buildEvalOutline,
    type EvalOutlineRow,
  } from '@/components/evals/eval-outline'
  import { groupReps } from '@/components/evals/group-reps'
  import type { RepSort } from '@/components/evals/sort-reps'
  import { indexEvalTraces } from '@/components/evals/eval-traces'
  import { exampleRuns } from '@/components/evals/examples'
  import { assembleEvalRunSource } from '@/services/eval-run-source'
  import type {
    EvalRunSourcePage,
    EvalRunSearchPage,
    EvalStoredRunSummary,
  } from '@/services/eval-run-types'
  import type { EvalRunCardData } from '@/components/evals/run-summary'
  import { getRouteContext } from '@/contexts/route-context.svelte'
  import { navigate, buildSearch, withQueryPatch } from '@/route'
  import { INGESTION_ISSUES_ID } from '@/components/shared/IngestionIssues.svelte'
  import type { ImportFailure } from '@/types/import-types'
  import { detailPanel } from '@/state/detail-panel.svelte'
  import type { TraceData } from '@/types/api-types'
  import type { EvalRun, EvalView } from '@/components/evals/model'
  import { onEvalRunsChanged } from '@/services/import-service'

  let {
    importFailure = null,
    onViewIssue,
  }: {
    importFailure?: ImportFailure | null
    onViewIssue?: (event: MouseEvent) => void
  } = $props()
  const route = getRouteContext()
  const query = $derived(route.route.query)
  const exampleMode = $derived(query.preview === 'examples')
  const databaseMode = $derived(!exampleMode)
  const localRuns = $derived(exampleMode ? exampleRuns : [])
  let storedRuns = $state<EvalStoredRunSummary[]>([])
  let storedFacets = $state<EvalStoredRunSummary['outcomes']>([])
  let runListTotal = $state(0)
  let runListHasMore = $state(false)
  let runListLoading = $state(false)
  let runListLoadingMore = $state(false)
  let runListError = $state('')
  let runListRevision = $state(0)
  let runListRequestVersion = 0
  let runIndexing = $state<EvalRunSearchPage['indexing']>({
    remaining: 0,
    unsupported: 0,
    empty: 0,
    issues: [],
    hasMoreIssues: false,
  })
  onMount(() => onEvalRunsChanged(() => (runListRevision += 1)))

  function storedRunCard(run: EvalStoredRunSummary): EvalRunCardData {
    return {
      id: run.id,
      name: run.name,
      evaluation: run.evaluation,
      source: run.source,
      state: run.state,
      stateReason: run.stateReason,
      startedAt: run.startedAt ?? undefined,
      pauseReason: undefined,
      outcomes: run.outcomes,
      outcomeLabel: run.assessmentName,
      repCount: run.repCount,
    }
  }

  function requestedOutcomes(value: string | undefined): string[] {
    if (!value) return []
    try {
      const parsed: unknown = JSON.parse(value)
      return Array.isArray(parsed)
        ? parsed.filter((item): item is string => typeof item === 'string')
        : []
    } catch {
      return []
    }
  }

  const localRunCards = $derived(localRuns.map(toRunCard))
  const databaseRunCards = $derived(storedRuns.map(storedRunCard))
  const outcomeOptions = $derived(
    databaseMode
      ? storedFacets.map(part => ({ ...part, key: part.key }))
      : runOutcomeOptions(localRunCards)
  )
  const selectedOutcomes = $derived(
    readRunOutcomeFilter(query.outcomes, outcomeOptions)
  )
  const filteredLocalRuns = $derived(
    selectedOutcomes.length
      ? localRunCards.filter(item =>
          item.outcomes.some(part =>
            selectedOutcomes.includes(outcomeKey(part))
          )
        )
      : localRunCards
  )
  const runSort: EvalRunSort = $derived(
    query.esort === 'name' ? 'name' : 'started'
  )
  const runSortDirection = $derived(query.edir === 'asc' ? 'asc' : 'desc')
  const visibleRuns = $derived(
    databaseMode
      ? databaseRunCards
      : [...filteredLocalRuns].sort((a, b) =>
          compareEvalRuns(a, b, runSort, runSortDirection)
        )
  )
  let storedRunSummary = $state<EvalStoredRunSummary>()
  let storedRun = $state<EvalRun>()
  let storedRunSource = $state<EvalSourceDataset>()
  let storedRunProjection = $state<EvalSourceProjection>()
  let storedRunLoading = $state(false)
  let storedRunError = $state('')

  const localRun = $derived(
    localRuns.find(item => item.id === query.run) ?? localRuns[0]
  )
  const run = $derived(databaseMode ? storedRun : localRun)
  const sourceCopy = $derived.by(() => {
    if (
      databaseMode &&
      storedRunSource &&
      storedRunProjection?.status === 'supported'
    )
      return evalSourceFieldCopy(
        storedRunSource.payload,
        storedRunProjection.rows
      )
  })
  const runCopy = $derived.by(() => {
    if (
      databaseMode &&
      storedRunSummary &&
      storedRunSource &&
      storedRunProjection?.status === 'supported'
    ) {
      const copy = evalSourceRunCopy(
        storedRunSource,
        storedRunProjection.runSources
      )
      return (
        _runID: string,
        field: 'name' | 'evaluation' | 'source' | 'prompt'
      ) => copy(storedRunSummary!.adapterRunID, field)
    }
  })
  const rep = $derived(run?.reps.find(item => item.id === query.rep))
  let resolvedRow = $state<EvalOutlineRow>()
  let traceViews = $state<TraceData[]>([])
  let telemetryLoading = $state(false)
  const telemetryNodes = $derived(indexEvalTraces(traceViews).nodes)
  let evalView = $state<EvalView>('results')
  let resultDisclosure = $state<Record<string, Record<string, boolean>>>({})
  let resultSorts = $state<Record<string, RepSort | undefined>>({})
  const fallbackOutline = $derived(
    run
      ? buildEvalOutline(
          groupReps(run.reps, run.reps),
          run,
          indexEvalTraces([])
        )
      : []
  )
  const detailRowID = $derived(
    query.row ??
      (rep
        ? (fallbackOutline.find(
            row => row.kind === 'case' && row.rep?.id === rep.id
          )?.id ?? `rep/${rep.id}`)
        : run
          ? `run/${run.id}`
          : undefined)
  )
  const selectedRow = $derived(
    detailRowID && resolvedRow?.id === detailRowID
      ? resolvedRow
      : detailRowID
        ? fallbackOutline.find(row => row.id === detailRowID)
        : undefined
  )
  const results = $derived.by(() => {
    if (!run) return []
    return run.reps
  })
  const selectedIndex = $derived(results.findIndex(item => item.id === rep?.id))
  const selectedRunIndex = $derived(
    visibleRuns.findIndex(item => item.id === (query.run ?? run?.id))
  )
  $effect(() => {
    const enabled = databaseMode
    const text = query.q
    const outcomes = requestedOutcomes(query.outcomes)
    const sort = runSort
    const direction = runSortDirection
    void runListRevision
    if (!enabled) {
      runListLoading = false
      return
    }
    const requestVersion = ++runListRequestVersion
    const controller = new AbortController()
    runListLoading = true
    runListError = ''
    storedRuns = []
    runListTotal = 0
    runListHasMore = false
    void (async () => {
      try {
        let page: EvalRunSearchPage
        do {
          page = await telemetryAPI.searchEvalRuns(
            {
              query: text,
              outcomes,
              sort,
              direction,
              limit: 100,
              offset: 0,
            },
            controller.signal
          )
          if (
            controller.signal.aborted ||
            requestVersion !== runListRequestVersion
          )
            return
          storedRuns = page.runs
          storedFacets = page.facets
          runListTotal = page.total
          runListHasMore = page.hasMore
          runIndexing = page.indexing
        } while (page.indexing.remaining > 0)
      } catch (error) {
        if (!controller.signal.aborted && !isAbortError(error))
          runListError =
            error instanceof Error
              ? error.message
              : 'Could not load experiments.'
      } finally {
        if (
          !controller.signal.aborted &&
          requestVersion === runListRequestVersion
        )
          runListLoading = false
      }
    })()
    return () => controller.abort()
  })

  $effect(() => {
    const enabled = databaseMode
    const id = query.run
    storedRun = undefined
    storedRunSummary = undefined
    storedRunSource = undefined
    storedRunProjection = undefined
    storedRunError = ''
    storedRunLoading = Boolean(enabled && id)
    if (!enabled || !id) return
    const controller = new AbortController()
    void (async () => {
      try {
        const pages: EvalRunSourcePage[] = []
        let after = -1
        while (true) {
          const page = await telemetryAPI.getEvalRunPage(
            id,
            after,
            100,
            controller.signal
          )
          if (controller.signal.aborted) return
          storedRunSummary = page.summary
          if (page.summary.repCount > evalProjectionLimits.rows)
            throw new Error(
              `This experiment has ${page.summary.repCount.toLocaleString()} results and exceeds the current 5,000-row display limit.`
            )
          pages.push(page)
          if (!page.hasMore) break
          after = page.nextAfter
        }
        const source = assembleEvalRunSource(pages)
        const projected = projectPromptfooDataset(source)
        if (projected.status !== 'supported') throw new Error(projected.reason)
        const projectedRun = projected.runs.find(
          item => item.id === pages[0].summary.adapterRunID
        )
        if (!projectedRun)
          throw new Error(
            'The selected experiment was not found in its source rows.'
          )
        const summary = pages[0].summary
        storedRunSource = source
        storedRunProjection = projected
        storedRun = {
          ...projectedRun,
          id: summary.id,
          name: summary.name,
          evaluation: summary.evaluation,
          source: summary.source,
          state: summary.state,
          stateReason: summary.stateReason,
          pauseReason: undefined,
          startedAt: summary.startedAt ?? undefined,
        }
      } catch (error) {
        if (!controller.signal.aborted && !isAbortError(error))
          storedRunError =
            error instanceof Error
              ? error.message
              : 'Could not load this experiment.'
      } finally {
        if (!controller.signal.aborted) storedRunLoading = false
      }
    })()
    return () => controller.abort()
  })

  $effect(() => {
    void run?.id
    resolvedRow = undefined
    traceViews = []
    telemetryLoading = false
  })

  // Store the first available run identity so reload/share preserves selection.
  $effect(() => {
    if (query.run) return
    if (databaseMode) {
      if (!runListLoading && storedRuns[0])
        update({ run: storedRuns[0].id }, true)
    } else if (run) update({ run: run.id }, true)
  })

  function update(changes: Record<string, string | null>, replace = false) {
    navigate(
      `/evals${buildSearch(withQueryPatch(query, changes))}`,
      replace ? 'replace' : 'push'
    )
  }
  function selectRun(id: string) {
    detailPanel.reveal()
    update({ run: id, rep: null, row: null })
  }
  function showDetail(rep: string | null) {
    if (!run) return
    detailPanel.reveal()
    update({ run: run.id, rep: rep, row: null })
  }
  function selectRow(row: EvalOutlineRow) {
    if (!run) return
    detailPanel.reveal()
    update({
      run: run.id,
      rep: row.rep?.id ?? null,
      row: row.kind === 'rep' ? null : row.id,
    })
  }
  function selectIndex(index: number) {
    const item = results[index]
    if (item) showDetail(item.id)
  }
  function selectRunIndex(index: number) {
    const item = visibleRuns[index]
    if (item) selectRun(item.id)
  }
  async function loadMoreRuns() {
    if (!databaseMode || !runListHasMore || runListLoadingMore) return
    const requestVersion = runListRequestVersion
    runListLoadingMore = true
    runListError = ''
    try {
      const page = await telemetryAPI.searchEvalRuns({
        query: query.q,
        outcomes: requestedOutcomes(query.outcomes),
        sort: runSort,
        direction: runSortDirection,
        limit: 100,
        offset: storedRuns.length,
      })
      if (requestVersion !== runListRequestVersion) return
      const seen = new Set(storedRuns.map(item => item.id))
      storedRuns = [
        ...storedRuns,
        ...page.runs.filter(item => !seen.has(item.id)),
      ]
      storedFacets = page.facets
      runListTotal = page.total
      runListHasMore = page.hasMore
      runIndexing = page.indexing
    } catch (error) {
      if (!isAbortError(error))
        runListError =
          error instanceof Error
            ? error.message
            : 'Could not load more experiments.'
    } finally {
      if (requestVersion === runListRequestVersion) runListLoadingMore = false
    }
  }
</script>

<PageLayout
  mainSurface="base"
  items={visibleRuns}
  navigationCount={databaseMode ? runListTotal : visibleRuns.length}
  selectedID={run?.id ?? query.run ?? null}
  loading={databaseMode && runListLoading && !visibleRuns.length}
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
  {#snippet drawerControls()}
    <ListHeaderControls label="Experiment controls">
      {#snippet sort()}
        <DrawerSortControl
          options={[
            { value: 'started', label: 'Started' },
            { value: 'name', label: 'Experiment name' },
          ]}
          value={runSort}
          direction={runSortDirection}
          onchange={(esort, edir) => update({ esort, edir })}
        />
      {/snippet}
      {#snippet filters()}
        <EvalOutcomeFilter
          options={outcomeOptions}
          selected={selectedOutcomes}
          onchange={keys =>
            update({ outcomes: keys.length ? JSON.stringify(keys) : null })}
        />
      {/snippet}
    </ListHeaderControls>
  {/snippet}
  {#snippet itemSnippet(item, selected)}
    <EvalRunSummary run={item} {selected} onclick={selectRun} />
  {/snippet}
  {#snippet drawerFooter()}
    <div class="flex justify-center">
      <DetailNav
        index={selectedRunIndex}
        total={visibleRuns.length}
        label="experiment"
        onFirst={() => selectRunIndex(0)}
        onPrev={() => selectRunIndex(selectedRunIndex - 1)}
        onNext={() => selectRunIndex(selectedRunIndex + 1)}
        onLast={() => selectRunIndex(visibleRuns.length - 1)}
      />
    </div>
    {#if (databaseMode && (runIndexing.remaining || runIndexing.unsupported || runIndexing.empty || runListHasMore)) || runListError}
      <div class="flex min-w-0 flex-col px-2 py-1 text-sm">
        {#if databaseMode && runIndexing.remaining > 0}<p
            class="px-2 py-1 text-rp-subtle"
            role="status"
          >
            Indexing saved evaluations… [{runIndexing.remaining}]
          </p>{/if}
        {#if databaseMode && (runIndexing.unsupported || runIndexing.empty)}<p
            class="px-2 py-1 text-rp-subtle"
          >
            {runIndexing.unsupported} unsupported · {runIndexing.empty} empty
          </p>{/if}
        {#if runListError}<p class="px-2 py-1" role="alert">
            {runListError}
          </p>{/if}
        {#if databaseMode && runListHasMore}<button
            type="button"
            class="btn btn-ghost btn-sm w-full font-normal"
            disabled={runListLoadingMore}
            onclick={loadMoreRuns}
            >{runListLoadingMore
              ? 'Loading experiments…'
              : 'Load more experiments'}</button
          >{/if}
      </div>
    {/if}
  {/snippet}
  {#snippet main()}
    {#if importFailure}<div class="alert alert-error">
        Import issue in {importFailure.fileName}.
        <a class="link" href={`/#${INGESTION_ISSUES_ID}`} onclick={onViewIssue}
          >View issue</a
        >
      </div>{/if}
    {#if databaseMode && storedRunLoading}
      <PaneHeader pill mode="title" title={storedRunSummary?.name ?? 'Evals'} />
      <p class="p-3 text-sm text-rp-subtle" role="status">
        Loading experiment…
      </p>
    {:else if databaseMode && storedRunError}
      <PaneHeader pill mode="title" title={storedRunSummary?.name ?? 'Evals'} />
      <p class="p-3 text-sm" role="alert">{storedRunError}</p>
    {:else if run}
      <EvalResults
        bind:viewKey={evalView}
        bind:disclosure={resultDisclosure}
        bind:sorts={resultSorts}
        {run}
        reps={results}
        selectedID={rep?.id}
        selectedRowID={detailRowID}
        onSelect={selectRow}
        onResolveSelection={row => (resolvedRow = row)}
        onTraceViews={(traces, loading) => {
          traceViews = traces
          telemetryLoading = loading
        }}
      />
      {#if !results.length}<p class="p-6 text-sm text-rp-subtle">
          No results recorded for this experiment.
        </p>{/if}
    {:else if runListLoading}
      <div class="m-[var(--layout-gap)] px-4 py-12 text-center text-rp-subtle">
        Loading evals…
      </div>
    {:else if !runListError}
      <div class="m-[var(--layout-gap)] px-4 py-12 text-center text-rp-subtle">
        <p>No evals found</p>
        <p class="mt-2 text-sm">
          Import an evaluation file or clear the search and filters
        </p>
      </div>
    {/if}
  {/snippet}
  {#snippet detail()}
    {#if run}
      {#key `${run.id}/${rep?.id ?? ''}/${selectedRow?.id ?? ''}`}
        {#if selectedRow && selectedRow.kind !== 'run'}<EvalRowDetail
            {run}
            row={selectedRow}
            {sourceCopy}
            mode={evalView}
            {telemetryNodes}
            {telemetryLoading}
          />{:else}<EvalDetail
            {run}
            row={selectedRow}
            storedSummary={databaseMode ? storedRunSummary : undefined}
            {runCopy}
            {telemetryNodes}
            {telemetryLoading}
          />{/if}
      {/key}
    {/if}
  {/snippet}
  {#snippet pageFooter()}
    <SignalFooter
      index={selectedIndex}
      total={results.length}
      label="result"
      onFirst={() => selectIndex(0)}
      onPrev={() => selectIndex(selectedIndex - 1)}
      onNext={() => selectIndex(selectedIndex + 1)}
      onLast={() => selectIndex(results.length - 1)}
    />
  {/snippet}
</PageLayout>

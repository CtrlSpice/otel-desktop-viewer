<script lang="ts">
  import { onDestroy, onMount } from 'svelte'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { Cancel01Icon } from '@hugeicons/core-free-icons'
  import { QuestionMarkIcon } from '@/icons'
  import PageLayout from '@/components/shared/PageLayout.svelte'
  import EvalDetailPlaceholder from '@/components/evals/EvalDetailPlaceholder.svelte'
  import PaneHeader from '@/components/shared/WorkspaceHeader.svelte'
  import DrawerSortControl from '@/components/shared/Drawer/DrawerSortControl.svelte'
  import ListHeaderControls from '@/components/shared/ListHeaderControls.svelte'
  import {
    compareEvalExperiments,
    type EvalExperimentSort,
  } from '@/components/evals/experiment-sort'
  import EvalExperimentSummary from '@/components/evals/EvalExperimentSummary.svelte'
  import EvalOutcomeFilter from '@/components/evals/EvalOutcomeFilter.svelte'
  import { toExperimentCard } from '@/components/evals/experiment-summary'
  import {
    outcomeKey,
    readExperimentOutcomeFilter,
    experimentOutcomeOptions,
  } from '@/components/evals/experiment-outcome-filter'
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
    evalSourceExperimentCopy,
  } from '@/components/evals/eval-source-copy'
  import { telemetryAPI, isAbortError } from '@/services/telemetry-service'
  import {
    buildEvalOutline,
    type EvalOutlineRow,
  } from '@/components/evals/eval-outline'
  import { groupResults } from '@/components/evals/group-results'
  import type { ResultSort } from '@/components/evals/sort-results'
  import {
    indexEvalTraces,
    projectEvalTraceEvidence,
  } from '@/components/evals/eval-traces'
  import { exampleExperiments } from '@/components/evals/examples'
  import { assembleEvalExperimentSource } from '@/services/eval-experiment-source'
  import type {
    EvalExperimentSourcePage,
    EvalExperimentSearchPage,
    EvalStoredExperimentSummary,
  } from '@/services/eval-experiment-types'
  import type { EvalExperimentCardData } from '@/components/evals/experiment-summary'
  import { getRouteContext } from '@/contexts/route-context.svelte'
  import { navigate, buildSearch, withQueryPatch } from '@/route'
  import { INGESTION_ISSUES_ID } from '@/components/shared/IngestionIssues.svelte'
  import type { ImportFailure } from '@/types/import-types'
  import { detailPanel } from '@/state/detail-panel.svelte'
  import type { LogData, TraceData } from '@/types/api-types'
  import type { EvalExperiment, EvalView } from '@/components/evals/model'
  import { onEvalExperimentsChanged } from '@/services/import-service'
  import { evalQuery } from '@/components/evals/eval-query'

  let {
    importFailure = null,
    onViewIssue,
  }: {
    importFailure?: ImportFailure | null
    onViewIssue?: (event: MouseEvent) => void
  } = $props()
  const route = getRouteContext()
  const query = $derived(evalQuery(route.route.query))
  const selectedExperimentID = $derived(query.experiment)
  const searchQuery = $derived(query.q ?? '')
  const outcomeQuery = $derived(query.outcomes)
  let searchText = $state('')
  let searchTimer: ReturnType<typeof setTimeout> | undefined
  $effect(() => {
    searchText = searchQuery
    clearTimeout(searchTimer)
  })
  onDestroy(() => clearTimeout(searchTimer))
  function search(value: string, immediate = false) {
    searchText = value
    clearTimeout(searchTimer)
    if (immediate) update({ q: value || null }, true)
    else searchTimer = setTimeout(() => update({ q: value || null }, true), 250)
  }
  $effect(() => {
    if (buildSearch(query) !== buildSearch(route.route.query))
      navigate(`/evals${buildSearch(query)}`, 'replace')
  })
  const exampleMode = $derived(query.preview === 'examples')
  const databaseMode = $derived(!exampleMode)
  const localExperiments = $derived(exampleMode ? exampleExperiments : [])
  let storedExperiments = $state<EvalStoredExperimentSummary[]>([])
  let storedFacets = $state<EvalStoredExperimentSummary['outcomes']>([])
  let experimentListHasMore = $state(false)
  let experimentListLoading = $state(false)
  let experimentListLoadingMore = $state(false)
  let experimentListError = $state('')
  let experimentListRevision = $state(0)
  let experimentListRequestVersion = 0
  let experimentIndexing = $state<EvalExperimentSearchPage['indexing']>({
    remaining: 0,
    unsupported: 0,
    empty: 0,
    issues: [],
    hasMoreIssues: false,
  })
  onMount(() => onEvalExperimentsChanged(() => (experimentListRevision += 1)))

  function storedExperimentCard(
    experiment: EvalStoredExperimentSummary
  ): EvalExperimentCardData {
    return {
      id: experiment.id,
      name: experiment.name,
      evaluation: experiment.evaluation,
      source: experiment.source,
      state: experiment.state,
      stateReason: experiment.stateReason,
      startedAt: experiment.startedAt ?? undefined,
      pauseReason: undefined,
      outcomes: experiment.outcomes,
      outcomeLabel: experiment.assessmentName,
      resultCount: experiment.resultCount,
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

  const localExperimentCards = $derived(localExperiments.map(toExperimentCard))
  const databaseExperimentCards = $derived(
    storedExperiments.map(storedExperimentCard)
  )
  const outcomeOptions = $derived(
    databaseMode
      ? storedFacets.map(part => ({ ...part, key: part.key }))
      : experimentOutcomeOptions(localExperimentCards)
  )
  const selectedOutcomes = $derived(
    readExperimentOutcomeFilter(query.outcomes, outcomeOptions)
  )
  const filteredLocalExperiments = $derived(
    selectedOutcomes.length
      ? localExperimentCards.filter(item =>
          item.outcomes.some(part =>
            selectedOutcomes.includes(outcomeKey(part))
          )
        )
      : localExperimentCards
  )
  const experimentSort: EvalExperimentSort = $derived(
    query.esort === 'name' ? 'name' : 'started'
  )
  const experimentSortDirection = $derived(
    query.edir === 'asc' ? 'asc' : 'desc'
  )
  const visibleExperiments = $derived(
    databaseMode
      ? databaseExperimentCards
      : [...filteredLocalExperiments].sort((a, b) =>
          compareEvalExperiments(a, b, experimentSort, experimentSortDirection)
        )
  )
  let storedExperimentSummary = $state<EvalStoredExperimentSummary>()
  let storedExperiment = $state<EvalExperiment>()
  let storedExperimentSource = $state<EvalSourceDataset>()
  let storedExperimentProjection = $state<EvalSourceProjection>()
  let storedExperimentLoading = $state(false)
  let storedExperimentError = $state('')
  let loadingNoticeReady = $state(false)
  const loadingResults = $derived(
    experimentListLoading || storedExperimentLoading
  )
  $effect(() => {
    loadingNoticeReady = false
    if (!loadingResults) return
    const timer = setTimeout(() => (loadingNoticeReady = true), 200)
    return () => clearTimeout(timer)
  })

  const localExperiment = $derived(
    localExperiments.find(item => item.id === query.experiment) ??
      localExperiments[0]
  )
  const experiment = $derived(databaseMode ? storedExperiment : localExperiment)
  const selectionPending = $derived(
    databaseMode &&
      Boolean(selectedExperimentID) &&
      experiment?.id !== selectedExperimentID
  )
  // Keep the displayed row with its experiment until the next source is ready.
  let displayedSelection = $state<{ result?: string; row?: string }>({})
  $effect(() => {
    if (!selectionPending)
      displayedSelection = { result: query.result, row: query.row }
  })
  const sourceCopy = $derived.by(() => {
    if (
      databaseMode &&
      storedExperimentSource &&
      storedExperimentProjection?.status === 'supported'
    )
      return evalSourceFieldCopy(
        storedExperimentSource.payload,
        storedExperimentProjection.rows
      )
  })
  const experimentCopy = $derived.by(() => {
    if (
      databaseMode &&
      storedExperimentSummary &&
      storedExperimentSource &&
      storedExperimentProjection?.status === 'supported'
    ) {
      const copy = evalSourceExperimentCopy(
        storedExperimentSource,
        storedExperimentProjection.experimentSources
      )
      return (
        _experimentID: string,
        field: 'name' | 'evaluation' | 'source' | 'prompt'
      ) => copy(storedExperimentSummary!.adapterExperimentID, field)
    }
  })
  const result = $derived(
    experiment?.results.find(item => item.id === displayedSelection.result)
  )
  let resolvedRow = $state<EvalOutlineRow>()
  let traceViews = $state<TraceData[]>([])
  let toolLogs = $state<LogData[]>([])
  let telemetryLoading = $state(false)
  const telemetryNodes = $derived(indexEvalTraces(traceViews).nodes)
  const detailExperiment = $derived(
    experiment
      ? projectEvalTraceEvidence(
          experiment,
          indexEvalTraces(traceViews),
          toolLogs
        )
      : undefined
  )
  let evalView = $state<EvalView>('results')
  let resultDisclosure = $state<Record<string, Record<string, boolean>>>({})
  let resultSorts = $state<Record<string, ResultSort | undefined>>({})
  const fallbackOutline = $derived(
    experiment
      ? buildEvalOutline(
          groupResults(experiment.results, experiment.results),
          experiment,
          indexEvalTraces([])
        )
      : []
  )
  const detailRowID = $derived(
    displayedSelection.row ??
      (result
        ? (fallbackOutline.find(
            row => row.kind === 'case' && row.result?.id === result.id
          )?.id ?? `result/${result.id}`)
        : experiment
          ? `experiment/${experiment.id}`
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
    if (!experiment) return []
    return experiment.results
  })
  const selectedIndex = $derived(
    results.findIndex(item => item.id === result?.id)
  )
  const selectedExperimentIndex = $derived(
    visibleExperiments.findIndex(
      item => item.id === (query.experiment ?? experiment?.id)
    )
  )
  $effect(() => {
    const enabled = databaseMode
    const text = searchQuery
    const outcomes = requestedOutcomes(outcomeQuery)
    const sort = experimentSort
    const direction = experimentSortDirection
    void experimentListRevision
    if (!enabled) {
      experimentListLoading = false
      return
    }
    const requestVersion = ++experimentListRequestVersion
    const controller = new AbortController()
    experimentListLoading = true
    experimentListLoadingMore = false
    experimentListError = ''
    experimentListHasMore = false
    void (async () => {
      try {
        let page: EvalExperimentSearchPage
        do {
          page = await telemetryAPI.searchEvalExperiments(
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
            requestVersion !== experimentListRequestVersion
          )
            return
          storedExperiments = page.experiments
          storedFacets = page.facets
          experimentListHasMore = page.hasMore
          experimentIndexing = page.indexing
        } while (page.indexing.remaining > 0)
      } catch (error) {
        if (!controller.signal.aborted && !isAbortError(error))
          experimentListError =
            error instanceof Error
              ? error.message
              : 'Could not load experiments.'
      } finally {
        if (
          !controller.signal.aborted &&
          requestVersion === experimentListRequestVersion
        )
          experimentListLoading = false
      }
    })()
    return () => controller.abort()
  })

  $effect(() => {
    const enabled = databaseMode
    const id = selectedExperimentID
    storedExperimentError = ''
    storedExperimentLoading = Boolean(enabled && id)
    if (!enabled || !id) return
    const controller = new AbortController()
    void (async () => {
      try {
        const pages: EvalExperimentSourcePage[] = []
        let after = -1
        while (true) {
          const page = await telemetryAPI.getEvalExperimentPage(
            id,
            after,
            100,
            controller.signal
          )
          if (controller.signal.aborted) return
          if (page.summary.resultCount > evalProjectionLimits.rows)
            throw new Error(
              `This experiment has ${page.summary.resultCount.toLocaleString()} results and exceeds the current 5,000-row display limit.`
            )
          pages.push(page)
          if (!page.hasMore) break
          after = page.nextAfter
        }
        const source = assembleEvalExperimentSource(pages)
        const projected = projectPromptfooDataset(source)
        if (projected.status !== 'supported') throw new Error(projected.reason)
        const projectedExperiment = projected.experiments.find(
          item => item.id === pages[0].summary.adapterExperimentID
        )
        if (!projectedExperiment)
          throw new Error(
            'The selected experiment was not found in its source rows.'
          )
        const summary = pages[0].summary
        storedExperimentSummary = summary
        storedExperimentSource = source
        storedExperimentProjection = projected
        storedExperiment = {
          ...projectedExperiment,
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
          storedExperimentError =
            error instanceof Error
              ? error.message
              : 'Could not load this experiment.'
      } finally {
        if (!controller.signal.aborted) storedExperimentLoading = false
      }
    })()
    return () => controller.abort()
  })

  $effect(() => {
    void experiment?.id
    resolvedRow = undefined
    traceViews = []
    toolLogs = []
    telemetryLoading = false
  })

  // Store the first available experiment identity so reload/share preserves selection.
  $effect(() => {
    if (query.experiment) return
    if (databaseMode) {
      if (!experimentListLoading && storedExperiments[0])
        update({ experiment: storedExperiments[0].id }, true)
    } else if (experiment) update({ experiment: experiment.id }, true)
  })

  function update(changes: Record<string, string | null>, replace = false) {
    navigate(
      `/evals${buildSearch(withQueryPatch(query, changes))}`,
      replace ? 'replace' : 'push'
    )
  }
  function selectExperiment(id: string) {
    detailPanel.reveal()
    update({ experiment: id, result: null, row: null })
  }
  function showDetail(result: string | null) {
    if (!experiment || selectionPending) return
    detailPanel.reveal()
    update({ experiment: experiment.id, result: result, row: null })
  }
  function selectRow(row: EvalOutlineRow) {
    if (!experiment || selectionPending) return
    detailPanel.reveal()
    update({
      experiment: experiment.id,
      result: row.result?.id ?? null,
      row: row.kind === 'result' ? null : row.id,
    })
  }
  function selectIndex(index: number) {
    const item = results[index]
    if (item) showDetail(item.id)
  }
  function selectExperimentIndex(index: number) {
    const item = visibleExperiments[index]
    if (item) selectExperiment(item.id)
  }
  async function loadMoreExperiments() {
    if (!databaseMode || !experimentListHasMore || experimentListLoadingMore)
      return
    const requestVersion = experimentListRequestVersion
    experimentListLoadingMore = true
    experimentListError = ''
    try {
      const page = await telemetryAPI.searchEvalExperiments({
        query: query.q,
        outcomes: requestedOutcomes(query.outcomes),
        sort: experimentSort,
        direction: experimentSortDirection,
        limit: 100,
        offset: storedExperiments.length,
      })
      if (requestVersion !== experimentListRequestVersion) return
      const seen = new Set(storedExperiments.map(item => item.id))
      storedExperiments = [
        ...storedExperiments,
        ...page.experiments.filter(item => !seen.has(item.id)),
      ]
      storedFacets = page.facets
      experimentListHasMore = page.hasMore
      experimentIndexing = page.indexing
    } catch (error) {
      if (
        requestVersion === experimentListRequestVersion &&
        !isAbortError(error)
      )
        experimentListError =
          error instanceof Error
            ? error.message
            : 'Could not load more experiments.'
    } finally {
      if (requestVersion === experimentListRequestVersion)
        experimentListLoadingMore = false
    }
  }
</script>

<PageLayout
  mainTitle="Evals"
  mainHeaderRows={2}
  mainHeaderInert={selectionPending}
  mainSurface="base"
  items={visibleExperiments}
  selectedID={selectedExperimentID ?? experiment?.id ?? null}
  loading={databaseMode && experimentListLoading && !visibleExperiments.length}
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
        value={searchText}
        oninput={event => search(event.currentTarget.value)}
        onkeydown={event => {
          if (event.key === 'Enter') search(event.currentTarget.value, true)
        }}
      />
      <button
        type="button"
        class="btn btn-circle btn-ghost btn-sm shrink-0 text-sm text-base-content"
        aria-label="Clear search"
        onclick={() => search('', true)}
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
  {#snippet drawerControls()}
    <ListHeaderControls label="Experiment controls">
      {#snippet sort()}
        <DrawerSortControl
          options={[
            { value: 'started', label: 'Started' },
            { value: 'name', label: 'Experiment name' },
          ]}
          value={experimentSort}
          direction={experimentSortDirection}
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
    <EvalExperimentSummary
      experiment={item}
      {selected}
      onclick={selectExperiment}
    />
  {/snippet}
  {#snippet drawerFooter()}
    <div class="flex justify-center">
      <DetailNav
        index={selectedExperimentIndex}
        total={visibleExperiments.length}
        label="experiment"
        onFirst={() => selectExperimentIndex(0)}
        onPrev={() => selectExperimentIndex(selectedExperimentIndex - 1)}
        onNext={() => selectExperimentIndex(selectedExperimentIndex + 1)}
        onLast={() => selectExperimentIndex(visibleExperiments.length - 1)}
      />
    </div>
    {#if (databaseMode && (experimentIndexing.remaining || experimentIndexing.unsupported || experimentIndexing.empty || experimentListHasMore)) || experimentListError}
      <div class="flex min-w-0 flex-col px-2 py-1 text-sm">
        {#if databaseMode && experimentIndexing.remaining > 0}<p
            class="px-2 py-1 text-subtle"
            role="status"
          >
            Indexing saved evaluations… [{experimentIndexing.remaining}]
          </p>{/if}
        {#if databaseMode && (experimentIndexing.unsupported || experimentIndexing.empty)}<p
            class="px-2 py-1 text-subtle"
          >
            {experimentIndexing.unsupported} unsupported · {experimentIndexing.empty}
            empty
          </p>{/if}
        {#if experimentListError}<p class="px-2 py-1" role="alert">
            {experimentListError}
          </p>{/if}
        {#if databaseMode && experimentListHasMore}<button
            type="button"
            class="btn btn-ghost btn-sm w-full font-normal"
            disabled={experimentListLoadingMore}
            onclick={loadMoreExperiments}
            >{experimentListLoadingMore
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
    {#if experiment}
      {#if databaseMode && storedExperimentLoading && loadingNoticeReady}
        <p class="px-3 py-1 text-sm text-subtle" role="status">
          Loading experiment… Showing {experiment.name} until it is ready.
        </p>
      {:else if databaseMode && storedExperimentError}
        <p class="px-3 py-1 text-sm" role="alert">
          {storedExperimentError} Still showing {experiment.name}.
        </p>
      {/if}
      <div
        class="flex min-h-0 flex-1 flex-col"
        aria-busy={databaseMode && storedExperimentLoading}
        inert={selectionPending}
      >
        <EvalResults
          bind:viewKey={evalView}
          bind:disclosure={resultDisclosure}
          bind:sorts={resultSorts}
          {experiment}
          {results}
          selectedID={result?.id}
          selectedRowID={detailRowID}
          onSelect={selectRow}
          onResolveSelection={row => (resolvedRow = row)}
          onTraceViews={(traces, loading, logs) => {
            traceViews = traces
            toolLogs = logs
            telemetryLoading = loading
          }}
        />
        {#if !results.length}<p class="p-6 text-sm text-subtle">
            No results recorded for this experiment.
          </p>{/if}
      </div>
    {:else if databaseMode && storedExperimentLoading}
      {#if loadingNoticeReady}
        <PaneHeader
          pill
          mode="title"
          title={storedExperimentSummary?.name ?? 'Evals'}
          reserveMetaRow
        />
        <p class="p-3 text-sm text-subtle" role="status">Loading experiment…</p>
      {/if}
    {:else if databaseMode && storedExperimentError}
      <PaneHeader
        pill
        mode="title"
        title={storedExperimentSummary?.name ?? 'Evals'}
        reserveMetaRow
      />
      <p class="p-3 text-sm" role="alert">{storedExperimentError}</p>
    {:else if experimentListLoading}
      {#if loadingNoticeReady}
        <div class="m-[var(--layout-gap)] px-4 py-12 text-center text-subtle">
          Loading evals…
        </div>
      {/if}
    {:else if !experimentListError}
      <div class="m-[var(--layout-gap)] px-4 py-12 text-center text-subtle">
        <p>No evals found</p>
        <p class="mt-2 text-sm">
          Import an evaluation file or clear the search and filters
        </p>
      </div>
    {/if}
  {/snippet}
  {#snippet detail()}
    {#if experiment}
      {@const rowDetails = selectedRow && selectedRow.kind !== 'experiment'}
      <div
        class="flex min-h-0 flex-1 flex-col"
        aria-busy={databaseMode && storedExperimentLoading}
        inert={selectionPending}
      >
        <div
          class="min-h-0 flex-1 flex-col"
          class:flex={!!rowDetails}
          class:hidden={!rowDetails}
        >
          {#if fallbackOutline[0]}<EvalRowDetail
              experiment={detailExperiment ?? experiment}
              row={selectedRow ?? fallbackOutline[0]}
              {sourceCopy}
              mode={evalView}
              {telemetryNodes}
              {telemetryLoading}
            />{/if}
        </div>
        <div
          class="min-h-0 flex-1 flex-col"
          class:flex={!rowDetails}
          class:hidden={!!rowDetails}
        >
          <EvalDetail
            experiment={detailExperiment ?? experiment}
            row={selectedRow?.kind === 'experiment' ? selectedRow : undefined}
            storedSummary={databaseMode ? storedExperimentSummary : undefined}
            {experimentCopy}
            {telemetryNodes}
            {telemetryLoading}
          />
        </div>
      </div>
    {:else}
      <EvalDetailPlaceholder />
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

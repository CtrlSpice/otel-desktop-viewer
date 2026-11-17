<script lang="ts">
  import { onDestroy, onMount, tick } from 'svelte'
  import { createFileImporter } from '@/services/import-service'
  import HomePage from '@/features/home/HomePage.svelte'
  import MetricsPage from '@/features/metrics/MetricsPage.svelte'
  import LogsPage from '@/features/logs/LogsPage.svelte'
  import TracesPage from '@/features/traces/TracesPage.svelte'
  import EvalsPage from '@/features/evals/EvalsPage.svelte'
  import FileDropOverlay from '@/components/shared/FileDropOverlay.svelte'
  import ImportFiles from '@/components/shared/ImportFiles.svelte'
  import { INGESTION_ISSUES_ID } from '@/components/shared/IngestionIssues.svelte'
  import type { ImportFailure } from '@/types/import-types'
  import { isPlainLeftClick, navigate, subscribeToRoute } from '@/route'
  import {
    createRouteContext,
    getRouteContext,
  } from '@/contexts/route-context.svelte'
  import { createTimeContext } from '@/contexts/time-context.svelte'
  import { createWorkspaceContext } from '@/contexts/workspace-context.svelte'
  import WorkspaceShell from '@/components/shared/Layout/WorkspaceShell.svelte'
  import { createNavigationCounts } from '@/contexts/navigation-counts.svelte'
  import { createDataChanges } from '@/contexts/data-changes.svelte'
  import {
    createImportNavigation,
    lookupImportTarget,
  } from '@/services/import-navigation'

  let {
    importFiles,
    importFailures = [],
  }: {
    importFiles?: (files: File[]) => void
    importFailures?: readonly ImportFailure[]
  } = $props()

  createRouteContext()
  createTimeContext()
  const changes = createDataChanges()
  createNavigationCounts()
  const workspace = createWorkspaceContext()

  let importMessage = $state('')
  let importRequestKey = $state(0)
  let localImportFailures = $state<ImportFailure[]>([])
  function reportImportFailure(failure: ImportFailure) {
    localImportFailures.push(failure)
  }
  const importNavigation = createImportNavigation({
    subscribe: changes.subscribe,
    lookup: lookupImportTarget,
    navigate(record, destination) {
      const query = new URLSearchParams({ time: 'all' })
      if (destination.spanID) query.set('span', destination.spanID)
      navigate(
        `/${record.signal}/${encodeURIComponent(destination.id)}?${query}`
      )
      void tick().then(() => changes.refreshViews())
    },
    status(message) {
      importMessage = message
    },
    onFailure: reportImportFailure,
  })
  onMount(() => {
    let previous = window.location.href
    return subscribeToRoute(() => {
      if (window.location.href === previous) return
      previous = window.location.href
      importNavigation.cancel()
    })
  })

  const importer = createFileImporter(window.location.href, reportImportFailure)
  const allImportFailures = $derived([
    ...importFailures,
    ...localImportFailures,
  ])
  function handleImportFiles(files: File[]) {
    if (importFiles) importFiles(files)
    else if (files.length) {
      importRequestKey++
      void importer.enqueue(files, importNavigation.select(files[0].name))
    }
  }
  onDestroy(() => {
    importNavigation.cancel()
    importer.cancel()
  })

  const routeContext = getRouteContext()
  let mainElement = $state<HTMLElement | null>(null)
  let importFailure = $state<ImportFailure | null>(null)
  let seenFailure: ImportFailure | undefined

  function pagePath(path: string): string {
    for (const base of ['/traces', '/metrics', '/logs', '/evals']) {
      if (path === base || path.startsWith(`${base}/`)) return base
    }
    return '/'
  }

  let lastFocusedPage = pagePath(routeContext.route.path)

  $effect(() => {
    const page = pagePath(routeContext.route.path)
    if (page === lastFocusedPage) return
    lastFocusedPage = page
    const focused = document.activeElement
    void tick().then(() => {
      if (pagePath(routeContext.route.path) !== page) return
      const issuePanel =
        window.location.hash === `#${INGESTION_ISSUES_ID}`
          ? document.getElementById(INGESTION_ISSUES_ID)
          : null
      if (issuePanel) {
        issuePanel.scrollIntoView({ block: 'nearest' })
        issuePanel.focus({ preventScroll: true })
      } else if (!focused?.isConnected || focused === document.body) {
        mainElement?.focus()
      }
    })
  })

  function under(base: string): boolean {
    const path = routeContext.route.path
    return path === base || path.startsWith(base + '/')
  }

  const Page = $derived(
    under('/traces')
      ? TracesPage
      : under('/metrics')
        ? MetricsPage
        : under('/logs')
          ? LogsPage
          : __EVALS_BUILT__ && under('/evals')
            ? EvalsPage
            : HomePage
  )

  $effect(() => {
    const latestFailure = allImportFailures.at(-1)
    if (latestFailure !== seenFailure) {
      seenFailure = latestFailure
      importFailure = Page === HomePage ? null : (latestFailure ?? null)
    }
    if (Page === HomePage) importFailure = null
  })

  function viewIssue(event: MouseEvent) {
    if (!isPlainLeftClick(event)) return
    event.preventDefault()
    importFailure = null
    navigate(`/#${INGESTION_ISSUES_ID}`)
  }
</script>

{#snippet emptyPane()}{/snippet}

<main
  bind:this={mainElement}
  tabindex="-1"
  class="flex h-screen min-w-0 flex-col overflow-hidden bg-base-100 transition-colors duration-300"
>
  {#if Page === HomePage}
    <HomePage importFailures={allImportFailures}>
      {#snippet importContent()}
        <ImportFiles onFiles={handleImportFiles} />
      {/snippet}
    </HomePage>
  {:else}
    <Page {importFailure} onViewIssue={viewIssue} />
  {/if}
  <WorkspaceShell
    {...workspace.props ?? {
      items: [],
      selectedID: null,
      drawerID: 'signal-drawer',
      drawerLabel: '',
      hideDrawer: Page === LogsPage || Page === HomePage,
      mainSurface: 'base',
      main: emptyPane,
      detail: emptyPane,
    }}
    loadingMessage={workspace.props?.loadingMessage || importMessage}
    loadingRequestKey={workspace.props?.loadingMessage
      ? workspace.props.loadingRequestKey
      : importMessage
        ? `import:${importRequestKey}`
        : undefined}
  />
</main>

<FileDropOverlay onFiles={handleImportFiles} />

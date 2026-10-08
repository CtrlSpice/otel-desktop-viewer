<script lang="ts">
  import { tick } from 'svelte'
  import HomePage from '@/pages/HomePage.svelte'
  import MetricsPage from '@/pages/MetricsPage.svelte'
  import LogsPage from '@/pages/LogsPage.svelte'
  import TracesPage from '@/pages/TracesPage.svelte'
  import FileDropOverlay from '@/components/shared/FileDropOverlay.svelte'
  import ImportFilesCard from '@/components/shared/ImportFilesCard.svelte'
  import ImportFailureNotice from '@/components/shared/ImportFailureNotice.svelte'
  import { INGESTION_ISSUES_ID } from '@/components/shared/IngestionIssues.svelte'
  import type { ImportFailure } from '@/types/import-types'
  import { isPlainLeftClick, navigate } from '@/route'
  import {
    createRouteContext,
    getRouteContext,
  } from '@/contexts/route-context.svelte'
  import { createTimeContext } from '@/contexts/time-context.svelte'

  let {
    importFiles,
    importFailures = [],
  }: {
    importFiles?: (files: File[]) => void
    importFailures?: readonly ImportFailure[]
  } = $props()

  createRouteContext()
  createTimeContext()

  const routeContext = getRouteContext()
  let mainElement = $state<HTMLElement | null>(null)
  let failureNotice = $state<ImportFailure | null>(null)
  let seenFailure: ImportFailure | undefined

  function pagePath(path: string): string {
    for (const base of ['/traces', '/metrics', '/logs']) {
      if (path === base || path.startsWith(`${base}/`)) return base
    }
    return '/'
  }

  let lastFocusedPage = pagePath(routeContext.route.path)

  $effect(() => {
    const page = pagePath(routeContext.route.path)
    if (page === lastFocusedPage) return
    lastFocusedPage = page
    void tick().then(() => {
      if (pagePath(routeContext.route.path) !== page) return
      const issuePanel =
        window.location.hash === `#${INGESTION_ISSUES_ID}`
          ? document.getElementById(INGESTION_ISSUES_ID)
          : null
      if (issuePanel) {
        issuePanel.scrollIntoView({ block: 'nearest' })
        issuePanel.focus({ preventScroll: true })
      } else {
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
          : HomePage
  )

  $effect(() => {
    const latestFailure = importFailures.at(-1)
    if (latestFailure !== seenFailure) {
      seenFailure = latestFailure
      failureNotice = Page === HomePage ? null : (latestFailure ?? null)
    }
    if (Page === HomePage) failureNotice = null
  })

  function viewIssue(event: MouseEvent) {
    if (!isPlainLeftClick(event)) return
    event.preventDefault()
    failureNotice = null
    navigate(`/#${INGESTION_ISSUES_ID}`)
  }
</script>

<main
  bind:this={mainElement}
  tabindex="-1"
  class="flex h-screen min-w-0 flex-col overflow-hidden bg-base-100 transition-colors duration-300"
>
  {#if Page === HomePage}
    <HomePage {importFailures}>
      {#snippet importContent()}
        {#if importFiles}
          <ImportFilesCard onFiles={importFiles} />
        {/if}
      {/snippet}
    </HomePage>
  {:else}
    <Page />
  {/if}
</main>

{#if importFiles}
  <FileDropOverlay onFiles={importFiles} />
{/if}

{#if failureNotice}
  <ImportFailureNotice
    failure={failureNotice}
    onViewIssue={viewIssue}
    onDismiss={() => (failureNotice = null)}
  />
{/if}

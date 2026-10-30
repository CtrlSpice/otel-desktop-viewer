<script module lang="ts">
  export type RawJsonScope = {
    id: string
    label: string
    url?: string
    previewURL?: string
    unavailable?: string
  }
</script>

<script lang="ts" generics="T">
  import { untrack } from 'svelte'
  import JsonEditor from './JsonEditor.svelte'
  import LoadingStatus from './LoadingStatus.svelte'
  import RawJsonStatus, { type RawJsonStatusData } from './RawJsonStatus.svelte'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import {
    Download01Icon,
    UnfoldLessIcon,
    UnfoldMoreIcon,
  } from '@hugeicons/core-free-icons'

  let {
    scopes,
    active,
    wholeURL,
    status = $bindable<RawJsonStatusData>(),
    showStatus = true,
    refreshKey,
    record,
    displayedRecord = $bindable<T | undefined>(),
  }: {
    scopes: RawJsonScope[]
    active: boolean
    wholeURL: string
    status?: RawJsonStatusData
    showStatus?: boolean
    refreshKey?: unknown
    record?: T
    displayedRecord?: T
  } = $props()
  const PREVIEW_BYTES = 2 * 1024 * 1024
  const formatDescription =
    'OpenTelemetry Protocol JSON. Size is the complete selected OTLP JSON in UTF-8 bytes before display formatting, not database storage. Reconstructed from retained telemetry, not the original request bytes. Includes resource and scope context; not restricted to the chart window.'
  let scopeID = $state(untrack(() => scopes[0]?.id ?? ''))
  let scope = $derived(scopes.find(item => item.id === scopeID) ?? scopes[0])
  let previewURL = $derived(scope?.previewURL ?? scope?.url)
  let displayedScope = $state<RawJsonScope>()
  let displayedBytes = $state<number | null>(null)
  let displayedWholeURL = $state('')
  let formatted = $state('')
  let completedURL = ''
  const visibleScope = $derived(formatted ? displayedScope : scope)
  let formatHint = $derived(
    visibleScope?.id === 'metric'
      ? `${formatDescription} Displayed metric JSON omits datapoints; Copy and Download include all retained datapoints.`
      : formatDescription
  )
  let bytes = $state<number | null>(null)
  let error = $state('')
  let inspectingSize = $state(false)
  let loadingPreview = $state(false)
  let loading = $derived(inspectingSize || loadingPreview)
  let partial = $state(false)
  let requestedURL = $state('')
  let currentURL = $state('')
  let currentRefreshKey: unknown
  let retry = $state(0)
  let requestKey = $state.raw<AbortSignal>()
  let editor = $state<{ collapseAll: () => void; expandAll: () => void }>()
  const sizeLabel = (size: number) =>
    size < 1024
      ? `${size} B`
      : size < 1024 * 1024
        ? `${(size / 1024).toFixed(1)} KiB`
        : `${(size / 1024 / 1024).toFixed(1)} MiB`

  $effect(() => {
    const size = formatted ? displayedBytes : bytes
    status = {
      label: `OTLP JSON${visibleScope?.id === 'metric' ? ' · without datapoints' : ''}${size === null ? '' : ` · ${sizeLabel(size)}`}`,
      description: formatHint,
      url: visibleScope?.previewURL ?? visibleScope?.url,
    }
  })

  $effect(() => {
    void retry
    const revision = refreshKey
    const url = previewURL
    if (
      url &&
      (!active ||
        (url === untrack(() => currentURL) && revision === currentRefreshKey))
    )
      return
    currentRefreshKey = revision
    currentURL = url ?? ''
    requestedURL = ''
    completedURL = ''
    error = ''
    bytes = null
    inspectingSize = false
    if (!url) {
      formatted = ''
      displayedRecord = undefined
      return
    }
    const selectedScope = scope
    const selectedRecord = record
    const controller = new AbortController()
    requestKey = controller.signal
    let inspected = false
    inspectingSize = true
    void fetch(url, { method: 'HEAD', signal: controller.signal })
      .then(async response => {
        if (controller.signal.aborted) return
        if (!response.ok)
          throw new Error(
            (await response.text()).trim() ||
              `Download unavailable (${response.status})`
          )
        const length = response.headers.get('Content-Length')
        bytes = length === null ? null : Number(length)
        // Metric metadata always loads a bounded preview without a manual gate.
        if (selectedScope?.id === 'metric') {
          requestedURL = url
        } else if (
          selectedScope?.id === 'datapoint' ||
          selectedScope?.id === 'span' ||
          selectedScope?.id === 'log'
        ) {
          if (bytes !== null && bytes <= PREVIEW_BYTES) requestedURL = url
        }
        if (!requestedURL) {
          // The manual preview gate is a completed result for this scope.
          formatted = ''
          displayedRecord = selectedRecord
        }
        inspected = true
      })
      .catch(err => {
        if (!controller.signal.aborted)
          error =
            err instanceof Error ? err.message : 'Could not inspect export size'
      })
      .finally(() => {
        if (!controller.signal.aborted) inspectingSize = false
      })
    return () => {
      controller.abort()
      inspectingSize = false
      if (!inspected) currentURL = ''
    }
  })

  $effect(() => {
    void refreshKey
    const url = requestedURL
    const selectedScope = scope
    const selectedRecord = record
    const selectedWholeURL = wholeURL
    const selectedBytes = bytes
    if (!active || !url || url !== previewURL) return
    if (untrack(() => formatted && completedURL === url)) return
    const controller = new AbortController()
    let worker: Worker | undefined
    loadingPreview = true
    error = ''
    void fetch(url, {
      signal: controller.signal,
      headers: { Range: `bytes=0-${PREVIEW_BYTES - 1}` },
    })
      .then(async response => {
        if (!response.ok)
          throw new Error(
            (await response.text()).trim() ||
              `Download unavailable (${response.status})`
          )
        // A server must honour the bound before the inspector accepts its body.
        const length = Number(response.headers.get('Content-Length'))
        if (!length || length > PREVIEW_BYTES)
          throw new Error(
            'This OTLP JSON cannot be previewed safely. Download the full OTLP JSON instead.'
          )
        const blob = await response.blob()
        if (controller.signal.aborted) return
        const buffer = await blob.arrayBuffer()
        if (controller.signal.aborted) return
        const previewPartial =
          response.status === 206 &&
          !!response.headers.get('Content-Range') &&
          !response.headers.get('Content-Range')!.endsWith(`/${blob.size}`)
        const raw = new TextDecoder().decode(buffer, {
          stream: previewPartial,
        })
        worker = new Worker(
          new URL('./utils/raw-json.worker.ts', import.meta.url),
          { type: 'module' }
        )
        worker.onmessage = (event: MessageEvent<string>) => {
          if (!controller.signal.aborted) {
            formatted = event.data
            partial = previewPartial
            completedURL = url
            displayedScope = selectedScope
            displayedRecord = selectedRecord
            displayedBytes = selectedBytes
            displayedWholeURL = selectedWholeURL
            loadingPreview = false
          }
          worker?.terminate()
        }
        worker.onerror = () => {
          if (!controller.signal.aborted) {
            error =
              'Could not format JSON. Download the exact OTLP JSON instead.'
            loadingPreview = false
          }
          worker?.terminate()
        }
        worker.postMessage(raw)
      })
      .catch(err => {
        if (!controller.signal.aborted) {
          error = err instanceof Error ? err.message : 'Could not load JSON'
          loadingPreview = false
        }
      })
    return () => {
      controller.abort()
      worker?.terminate()
      loadingPreview = false
    }
  })
</script>

<section class="raw-inspector" aria-label="Raw JSON inspector">
  <div class="raw-inspector__controls">
    {#if scopes.length > 1}<label class="flex items-center gap-2 text-sm"
        >Scope
        <select
          class="select select-sm text-sm"
          aria-label="Raw JSON scope"
          bind:value={scopeID}
        >
          {#each scopes as item (item.id)}<option value={item.id}
              >{item.label}</option
            >{/each}
        </select>
      </label>{/if}
    {#if showStatus && status}<RawJsonStatus {status} />{/if}
    <div
      class="header-icon-controls ml-auto shrink-0"
      role="group"
      aria-label="JSON folding"
    >
      <button
        type="button"
        class="btn btn-xs chart-header-control tooltip tooltip-left"
        aria-label="Collapse JSON"
        data-tip="Collapse JSON"
        disabled={!editor}
        onclick={() => editor?.collapseAll()}
      >
        <HugeiconsIcon
          icon={UnfoldLessIcon}
          size="1em"
          strokeWidth={1.5}
          class="h-4 w-4"
          aria-hidden="true"
        />
      </button>
      <button
        type="button"
        class="btn btn-xs chart-header-control tooltip tooltip-left"
        aria-label="Expand JSON"
        data-tip="Expand JSON"
        disabled={!editor}
        onclick={() => editor?.expandAll()}
      >
        <HugeiconsIcon
          icon={UnfoldMoreIcon}
          size="1em"
          strokeWidth={1.5}
          class="h-4 w-4"
          aria-hidden="true"
        />
      </button>
    </div>
    {#if visibleScope?.url && visibleScope.url !== (formatted ? displayedWholeURL : wholeURL)}<a
        class="header-icon-button tooltip tooltip-left"
        href={visibleScope.url}
        aria-label="Download {visibleScope.label.toLowerCase()}"
        data-tip="Download {visibleScope.label.toLowerCase()}"
        download
        ><HugeiconsIcon
          icon={Download01Icon}
          size={16}
          strokeWidth={1.5}
          aria-hidden="true"
        /></a
      >{/if}
  </div>
  {#if !scope?.url}<p class="p-3 text-sm">
      {scope?.unavailable ?? 'Select a record to inspect its raw JSON.'}
    </p>
  {:else if error}<p class="p-3 text-sm text-error" role="alert">{error}</p>
    <button
      type="button"
      class="btn btn-ghost btn-sm"
      onclick={() => {
        currentURL = ''
        requestedURL = ''
        retry++
      }}>Retry</button
    >
  {:else if loading}<LoadingStatus
      pending
      label="Loading JSON…"
      {requestKey}
      class="block p-3 text-sm text-subtle"
    />
  {:else if !formatted && scope?.id !== 'metric'}<div class="p-3">
      <button
        type="button"
        class="btn btn-soft btn-sm text-sm"
        onclick={() => (requestedURL = previewURL!)}
        >{bytes !== null && bytes > PREVIEW_BYTES
          ? 'Load 2 MiB preview'
          : `Load ${scope.label.toLowerCase()} JSON`}</button
      >
    </div>{/if}
  {#if formatted}
    {#if partial}<p class="raw-inspector__notice" role="status">
        Partial preview: first 2 MiB only. Search applies only to this preview.
        The JSON may end mid-value. Download contains the complete selected
        scope.
      </p>{/if}
    <JsonEditor raw label="Raw OTLP JSON" text={formatted} bind:this={editor} />
  {/if}
</section>

<style lang="postcss">
  @reference "../../app.css";
  .raw-inspector {
    @apply min-w-0 py-2;
  }
  .raw-inspector__controls {
    @apply flex flex-wrap items-center gap-2 px-2;
  }
  .raw-inspector__controls .chart-header-control {
    @apply rounded-full;
    width: var(--header-control-height, 2rem);
    padding-inline: 0;
  }
  .raw-inspector__notice {
    @apply m-2 rounded-lg bg-warning/10 p-3 text-sm text-base-content;
  }
</style>

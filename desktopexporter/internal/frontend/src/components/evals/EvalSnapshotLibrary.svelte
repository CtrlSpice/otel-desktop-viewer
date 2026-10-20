<script lang="ts">
  import { onDestroy } from 'svelte'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { Download01Icon } from '@hugeicons/core-free-icons'
  import Identifier from '@/components/shared/Identifier.svelte'
  import JsonEditor from '@/components/shared/JsonEditor.svelte'
  import {
    telemetryAPI,
    isAbortError,
    evalSnapshotDownloadURL,
    downloadEvalSnapshot,
    EVAL_SNAPSHOT_MAX_BYTES,
    type EvalSnapshot,
    type EvalSnapshotFormat,
    type EvalSnapshotMetadata,
  } from '@/services/telemetry-service'
  import type { SnapshotProjection } from './snapshot-adapters'

  let {
    snapshot,
    projection,
    snapshotID,
    loading = false,
    error = '',
    sourcePointer,
    onOpen,
    onDelete,
    onSynthetic,
  }: {
    snapshot?: EvalSnapshot
    projection?: SnapshotProjection
    snapshotID?: string
    loading?: boolean
    error?: string
    sourcePointer?: string
    onOpen: (id: string) => void
    onDelete: (id: string) => void
    onSynthetic: () => void
  } = $props()

  const pageSize = 25
  const previewSize = 32768
  let expanded = $state(false)
  let snapshots = $state<EvalSnapshotMetadata[]>([])
  let hasMore = $state(false)
  let offset = $state(0)
  let revision = $state(0)
  let listing = $state(false)
  let listError = $state('')
  let actionError = $state('')
  let message = $state('')
  let format = $state<EvalSnapshotFormat | ''>('')
  let source = $state('local-file')
  let file = $state<File>()
  let busy = $state(false)
  let mutation: AbortController | undefined
  let previewOffset = $state(0)
  const preview = $derived(
    snapshot?.originalJSON.slice(previewOffset, previewOffset + previewSize) ??
      ''
  )

  $effect(() => {
    void snapshot?.id
    previewOffset = 0
  })

  $effect(() => {
    if (!expanded) return
    const pageOffset = offset
    void revision
    const controller = new AbortController()
    listing = true
    listError = ''
    snapshots = []
    hasMore = false
    telemetryAPI
      .searchEvalSnapshots(pageSize, pageOffset, controller.signal)
      .then(page => {
        if (controller.signal.aborted) return
        snapshots = page.snapshots
        hasMore = page.hasMore
      })
      .catch(error => {
        if (!controller.signal.aborted && !isAbortError(error))
          listError =
            error instanceof Error ? error.message : 'Could not load snapshots.'
      })
      .finally(() => {
        if (!controller.signal.aborted) listing = false
      })
    return () => controller.abort()
  })

  onDestroy(() => mutation?.abort())

  function cancel() {
    mutation?.abort()
    busy = false
    message =
      'Request canceled. Refresh the archive to check whether it completed.'
  }

  async function importFile() {
    const selectedFile = file
    if (!selectedFile || !format || busy) return
    const selectedFormat = format
    const selectedSource = source
    const controller = new AbortController()
    mutation = controller
    busy = true
    actionError = ''
    message = ''
    try {
      if (selectedFile.size > EVAL_SNAPSHOT_MAX_BYTES)
        throw new Error('Snapshot exceeds the 16 MiB file limit.')
      // Fatal decoding rejects invalid UTF-8. ignoreBOM preserves a BOM for the
      // backend to reject instead of silently changing the original file bytes.
      const originalJSON = new TextDecoder('utf-8', {
        fatal: true,
        ignoreBOM: true,
      }).decode(await selectedFile.arrayBuffer())
      if (controller.signal.aborted) return
      const result = await telemetryAPI.importEvalSnapshot(
        selectedSource,
        selectedFormat,
        selectedFile.name,
        originalJSON,
        controller.signal
      )
      if (controller.signal.aborted) return
      message = result.created
        ? `Archived ${result.snapshot.name}. Open it from the library.`
        : `Already archived as ${result.snapshot.name}; original metadata retained.`
      offset = 0
      revision += 1
    } catch (error) {
      if (!controller.signal.aborted && !isAbortError(error))
        actionError =
          error instanceof Error ? error.message : 'Could not import snapshot.'
    } finally {
      if (!controller.signal.aborted) busy = false
    }
  }

  async function remove(id: string) {
    if (busy) return
    const controller = new AbortController()
    mutation = controller
    busy = true
    actionError = ''
    message = ''
    try {
      await telemetryAPI.deleteEvalSnapshot(id, controller.signal)
      if (controller.signal.aborted) return
      onDelete(id)
      message = 'Snapshot deleted.'
      offset = 0
      revision += 1
    } catch (error) {
      if (!controller.signal.aborted && !isAbortError(error))
        actionError =
          error instanceof Error ? error.message : 'Could not delete snapshot.'
    } finally {
      if (!controller.signal.aborted) busy = false
    }
  }

  async function download(event: MouseEvent, id: string) {
    event.preventDefault()
    if (busy) return
    const controller = new AbortController()
    mutation = controller
    busy = true
    actionError = ''
    message = ''
    try {
      await downloadEvalSnapshot(id, controller.signal)
    } catch (error) {
      if (!controller.signal.aborted && !isAbortError(error))
        actionError =
          error instanceof Error
            ? error.message
            : 'Could not download snapshot.'
    } finally {
      if (!controller.signal.aborted) busy = false
    }
  }
</script>

<section class="shrink-0 text-sm" aria-label="Eval snapshot archive">
  <div class="flex min-w-0 flex-wrap items-center gap-2 px-3 py-2">
    <button
      class="btn btn-ghost btn-sm font-normal"
      aria-expanded={expanded}
      aria-controls="eval-snapshot-library"
      onclick={() => {
        expanded = !expanded
        if (!expanded) file = undefined
      }}>Snapshot library</button
    >
    {#if snapshotID}
      <button class="btn btn-ghost btn-sm font-normal" onclick={onSynthetic}
        >Synthetic preview</button
      >
    {:else}
      <span class="text-rp-subtle">Synthetic preview</span>
    {/if}
    {#if snapshot}
      <span class="min-w-0 flex-1 truncate" title={snapshot.name}
        >{snapshot.name}</span
      >
      <button
        class="btn btn-ghost btn-sm font-normal"
        disabled={busy}
        onclick={() => remove(snapshot.id)}>Delete snapshot</button
      >
      <a
        class="header-icon-button"
        href={evalSnapshotDownloadURL(snapshot.id)}
        onclick={event => download(event, snapshot.id)}
        aria-disabled={busy}
        download
        title="Download original snapshot JSON"
        aria-label="Download original snapshot JSON"
        ><HugeiconsIcon
          icon={Download01Icon}
          size={17}
          strokeWidth={1.5}
          aria-hidden="true"
        /></a
      >
    {/if}
  </div>
  {#if loading}<p class="px-3 py-2 text-rp-subtle" role="status">
      Loading snapshot…
    </p>{/if}
  {#if error}<p class="px-3 py-2" role="alert">{error}</p>{/if}
  {#if actionError}<p class="px-3 py-2" role="alert">{actionError}</p>{/if}
  {#if message}<p class="px-3 py-2 text-rp-subtle" role="status">
      {message}
    </p>{/if}
  {#if busy}<button
      class="btn btn-ghost btn-sm mx-3 font-normal"
      onclick={cancel}>Cancel request</button
    >{/if}

  {#if expanded}
    <div
      id="eval-snapshot-library"
      class="max-h-96 space-y-2 overflow-auto px-3 py-2"
    >
      <h2 class="text-base font-normal">Snapshot library</h2>
      <form
        aria-label="Import eval snapshot"
        class="flex flex-wrap items-end gap-2"
        onsubmit={event => {
          event.preventDefault()
          void importFile()
        }}
      >
        <label class="flex min-w-0 flex-col gap-1"
          ><span class="text-rp-subtle">JSON file · up to 16 MiB</span><input
            class="file-input min-w-0 max-w-full text-sm"
            type="file"
            accept="application/json,.json"
            onchange={event => (file = event.currentTarget.files?.[0])}
            disabled={busy}
            required
          /></label
        >
        <label class="flex flex-col gap-1"
          ><span class="text-rp-subtle">Source format</span><select
            class="select text-sm"
            bind:value={format}
            disabled={busy}
            required
            ><option value="" disabled>Select format…</option><option
              value="promptfoo-v3">promptfoo-v3</option
            ><option value="viewer-kit">viewer-kit</option><option
              value="unknown-json">unknown-json</option
            ></select
          ></label
        >
        <label class="flex flex-col gap-1"
          ><span class="text-rp-subtle">Source label</span><input
            class="input text-sm"
            bind:value={source}
            disabled={busy}
            required
          /></label
        >
        <button
          class="btn btn-ghost btn-sm font-normal"
          type="submit"
          disabled={busy || !file || !format}>Import snapshot</button
        >
      </form>
      <p class="text-rp-subtle">
        Original JSON is authoritative. Imports retain all fields and file text.
        The escaped request must fit within 20 MiB.
      </p>
      <div class="flex flex-wrap items-center gap-2">
        <button
          class="btn btn-ghost btn-sm font-normal"
          onclick={() => (revision += 1)}
          disabled={listing}>Refresh archive</button
        >
        <button
          class="btn btn-ghost btn-sm font-normal"
          disabled={listing || offset === 0}
          onclick={() => (offset -= pageSize)}>Previous page</button
        >
        <span class="text-rp-subtle"
          >Page {offset / pageSize + 1} · {snapshots.length} snapshots</span
        >
        <button
          class="btn btn-ghost btn-sm font-normal"
          disabled={listing || !hasMore || offset + pageSize > 1000000}
          onclick={() => (offset += pageSize)}>Next page</button
        >
      </div>
      {#if listError}<p role="alert">{listError}</p>{/if}
      {#if listing}<p role="status" class="text-rp-subtle">
          Loading archive…
        </p>{:else if !snapshots.length && !listError}<p class="text-rp-subtle">
          No snapshots on this page.
        </p>{/if}
      <ul>
        {#each snapshots as item (item.id)}
          <li class="flex min-w-0 flex-wrap items-center gap-2 py-1">
            <button
              class="btn btn-ghost btn-sm min-w-0 font-normal"
              aria-label={`Open snapshot ${item.name}`}
              onclick={() => onOpen(item.id)}>{item.name}</button
            >
            <span class="text-rp-subtle"
              >{item.format} · {item.source} · {item.byteSize.toLocaleString()} bytes</span
            >
            <span class="text-rp-subtle"><Identifier value={item.id} /></span>
            <span class="flex-1"></span>
            <button
              class="btn btn-ghost btn-sm font-normal"
              disabled={busy}
              aria-label={`Delete snapshot ${item.name}`}
              onclick={() => remove(item.id)}>Delete</button
            >
            <a
              class="header-icon-button"
              href={evalSnapshotDownloadURL(item.id)}
              onclick={event => download(event, item.id)}
              aria-disabled={busy}
              download
              title={`Download original JSON for ${item.name}`}
              aria-label={`Download original JSON for ${item.name}`}
              ><HugeiconsIcon
                icon={Download01Icon}
                size={17}
                strokeWidth={1.5}
                aria-hidden="true"
              /></a
            >
          </li>
        {/each}
      </ul>
    </div>
  {/if}

  {#if snapshot}
    <details class="px-3 py-2">
      <summary class="cursor-pointer"
        >Original JSON and projection context</summary
      >
      <div class="max-h-96 space-y-2 overflow-auto py-2">
        <p class="text-rp-subtle">
          {snapshot.format} · {snapshot.source} · {snapshot.byteSize.toLocaleString()}
          bytes · <Identifier value={snapshot.id} />
        </p>
        {#if projection?.status === 'supported'}
          <p class="text-rp-subtle">
            {projection.runs.length} projected runs · Original JSON is authoritative.
          </p>
        {:else if projection}<p>{projection.reason}</p>{/if}
        {#each projection?.notes ?? [] as note}<p class="text-rp-subtle">
            {note}
          </p>{/each}
        {#if sourcePointer}<p>
            Selected result JSON Pointer: {sourcePointer}
          </p>{/if}
        <div class="flex flex-wrap items-center gap-2">
          <button
            class="btn btn-ghost btn-sm font-normal"
            disabled={previewOffset === 0}
            onclick={() => (previewOffset -= previewSize)}
            >Previous JSON section</button
          >
          <span class="text-rp-subtle"
            >Original text characters {previewOffset + 1}–{Math.min(
              previewOffset + previewSize,
              snapshot.originalJSON.length
            )} of {snapshot.originalJSON.length}</span
          >
          <button
            class="btn btn-ghost btn-sm font-normal"
            disabled={previewOffset + previewSize >=
              snapshot.originalJSON.length}
            onclick={() => (previewOffset += previewSize)}
            >Next JSON section</button
          >
        </div>
        <div class="bg-base-100">
          <JsonEditor
            text={preview}
            raw
            label="Original snapshot JSON section"
          />
        </div>
      </div>
    </details>
    {#if projection && projection.status !== 'supported'}<p class="px-3 py-2">
        {projection.reason} Inspect the original JSON or download the snapshot.
      </p>{/if}
  {/if}
</section>

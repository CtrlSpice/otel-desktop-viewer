<script lang="ts">
  import { onMount, onDestroy, type Snippet } from 'svelte'
  import {
    EditorView,
    placeholder as cmPlaceholder,
    tooltips,
  } from '@codemirror/view'
  import { EditorState, Compartment } from '@codemirror/state'
  import { autocompletion, closeBrackets } from '@codemirror/autocomplete'
  import { history, defaultKeymap, historyKeymap } from '@codemirror/commands'
  import { keymap } from '@codemirror/view'
  import {
    getStaticFieldsForSearch,
    getDynamicAttributes,
    type FieldDefinition,
  } from '@/constants/fields'
  import { parseQuery } from './queryParser'
  import type { QueryNode } from './queryTree'
  import { telemetryAPI } from '@/services/telemetry-service'
  import {
    getTimeContext,
    selectionToQueryRangeMs,
  } from '@/contexts/time-context.svelte'
  import type { TimeContext } from '@/contexts/time-context.svelte'
  import type { SearchResultEvent } from '@/types/api-types'
  import type { FilterDescriptor } from '@/components/SignalToolbar/filter-types'
  import { queryLanguageSupport } from './lang/query-language'
  import { createQueryCompletionSource } from './lang/completions'
  import { createQueryLinter } from './lang/linter'
  import { queryTheme, ensureTooltipStyles } from './lang/theme'
  import { createQueryKeymap } from './lang/keymap'
  import { HelpCircleIcon, CancelIcon } from '@/icons'
  import FieldErrorMessage from '@/components/FieldErrorMessage.svelte'

  import type { SearchEditorAPI } from './search-editor-api'
  import { parseDuration } from '@/utils/time'

  // --- types ---
  type SearchEditorProps = {
    signal: 'traces' | 'metrics' | 'logs'
    onSearchResults?: (event: SearchResultEvent) => void
    /** Toolbar layout: panel chrome on the search wrapper only (not the action row). */
    inToolbar?: boolean
    /** 'drawer' moves action buttons to a footer row below the editor. */
    variant?: 'default' | 'drawer'
    /** Called once after mount with imperative handles. */
    onReady?: (api: SearchEditorAPI) => void
    /** Called whenever the search error state changes. */
    onSearchError?: (error: string | null) => void
    /** Actions pinned top-right inside the editor container (drawer variant only). */
    headerActions?: Snippet
  }

  // --- helpers ---

  type SearchContext = {
    signal: 'traces' | 'logs' | 'metrics'
    startTime: number
    endTime: number
  }

  const searchDispatch: Record<
    string,
    (ctx: SearchContext, q?: QueryNode) => () => Promise<any>
  > = {
    traces: (ctx, q) => () =>
      telemetryAPI.searchTraces(ctx.startTime, ctx.endTime, q),
    logs: (ctx, q) => () =>
      telemetryAPI.searchLogs(ctx.startTime, ctx.endTime, q),
    metrics: (ctx, q) => () =>
      telemetryAPI.getMetrics(ctx.startTime, ctx.endTime, q),
  }

  /** Build the API call for a signal, or null if unsupported. */
  function buildSearchFn(
    ctx: SearchContext,
    queryTree?: QueryNode
  ): (() => Promise<any>) | null {
    return searchDispatch[ctx.signal]?.(ctx, queryTree) ?? null
  }

  /**
   * Walk the query tree and convert human-readable duration values to
   * nanosecond strings in-place. Returns an error message if any
   * duration value can't be parsed, or null on success.
   */
  function normalizeDurationValues(node: QueryNode): string | null {
    if (node.type === 'group') {
      return node.group.children.reduce<string | null>(
        (err, child) => err ?? normalizeDurationValues(child),
        null
      )
    }
    if (!('name' in node.query.field) || node.query.field.name !== 'duration')
      return null

    const ns = parseDuration(node.query.value)
    if (ns === null)
      return `Invalid duration: "${node.query.value}". Try "1s", "500ms", "2m", etc.`
    node.query.value = ns.toString()
    return null
  }

  const capitalize = (s: string) => s.charAt(0).toUpperCase() + s.slice(1)

  // --- context ---
  let {
    signal,
    onSearchResults,
    inToolbar = false,
    variant = 'default',
    onReady,
    onSearchError,
    headerActions,
    filters = [],
  }: SearchEditorProps & { filters?: FilterDescriptor[] } = $props()

  const isMac =
    typeof navigator !== 'undefined' &&
    /Mac|iPhone|iPad/.test(navigator.userAgent)
  const modKey = isMac ? '⌘' : 'Ctrl'

  let timeContext: TimeContext | null = null
  try {
    timeContext = getTimeContext()
  } catch {
    console.warn('SearchEditor: time context not available')
  }

  // --- state: editor ---
  let editorContainer = $state<HTMLDivElement | null>(null)
  let editorView: EditorView | null = null
  let searchError = $state<string | null>(null)
  const placeholderCompartment = new Compartment()

  // --- state: filter popover ---
  let activeFilterId = $state<string | null>(null)
  let filterPopoverEl = $state<HTMLDivElement | null>(null)

  let activeFilter = $derived(
    activeFilterId ? (filters.find(f => f.id === activeFilterId) ?? null) : null
  )

  // --- state: help dialog ---
  let helpDialogElement = $state<HTMLDialogElement | null>(null)
  let helpDialogOpen = $state(false)
  let helpLastFocusedElement = $state<HTMLElement | null>(null)
  let prevBodyOverflow = $state<string | null>(null)
  const supportsClosedBy = 'closedBy' in HTMLDialogElement.prototype

  // --- state: available fields (static + dynamic attributes) ---
  let availableFields = $state<FieldDefinition[]>([])

  // --- derived ---
  let signalLabel = $derived(capitalize(signal))
  let staticFieldsList = $derived([...getStaticFieldsForSearch(signal)])
  let placeholderText = $derived(`search ${signal}...`)

  // --- effects ---

  $effect(() => {
    editorView?.dispatch({
      effects: placeholderCompartment.reconfigure(
        cmPlaceholder(placeholderText)
      ),
    })
  })

  /** Fetch dynamic attributes by time range. */
  $effect(() => {
    const base = [...staticFieldsList]
    availableFields = base

    const tc = timeContext
    if (!tc) return

    let cancelled = false
    const t = window.setTimeout(async () => {
      try {
        const { start, end } = selectionToQueryRangeMs(tc.selection, Date.now())
        const dynamicAttrs = await getDynamicAttributes(start, end, signal)
        if (cancelled) return
        availableFields = [...base, ...dynamicAttrs]
      } catch (error) {
        console.warn('Failed to load dynamic attributes:', error)
      }
    }, 250)

    return () => {
      cancelled = true
      window.clearTimeout(t)
    }
  })

  /** Wire up close/cancel/click-outside on the help dialog element. */
  $effect(() => {
    if (!helpDialogElement) return

    const dialog = helpDialogElement

    const handleClose = () => {
      helpDialogOpen = dialog.open
      if (prevBodyOverflow !== null) {
        document.body.style.overflow = prevBodyOverflow
        prevBodyOverflow = null
      }
      if (helpLastFocusedElement?.isConnected) {
        helpLastFocusedElement.focus()
      }
      helpLastFocusedElement = null
    }

    const handleCancel = () => {
      helpDialogOpen = false
    }

    const handleClickOutside = (event: MouseEvent) => {
      if (supportsClosedBy) return
      const rect = dialog.getBoundingClientRect()
      const inside =
        rect.top <= event.clientY &&
        event.clientY <= rect.top + rect.height &&
        rect.left <= event.clientX &&
        event.clientX <= rect.left + rect.width
      if (!inside) dialog.close()
    }

    dialog.addEventListener('close', handleClose)
    dialog.addEventListener('cancel', handleCancel)
    if (!supportsClosedBy) {
      dialog.addEventListener('click', handleClickOutside)
    }

    helpDialogOpen = dialog.open

    return () => {
      dialog.removeEventListener('close', handleClose)
      dialog.removeEventListener('cancel', handleCancel)
      if (!supportsClosedBy) {
        dialog.removeEventListener('click', handleClickOutside)
      }
    }
  })

  // --- handlers ---

  /** Build a SearchContext from the current component state. */
  function currentSearchContext(): SearchContext {
    const { start: startTime, end: endTime } = timeContext
      ? selectionToQueryRangeMs(timeContext.selection, Date.now())
      : { start: 0, end: Date.now() }

    return { signal, startTime, endTime }
  }

  /** Fetch without any search filter and deliver via onSearchResults. */
  function fetchClean() {
    const ctx = currentSearchContext()
    const fn = buildSearchFn(ctx)
    fn?.()
      .then(results => {
        emitResults(results)
      })
      .catch(err => {
        searchError = 'Search failed: ' + err.message
      })
  }

  /** Emit results with the query tree attached so consumers can reuse it. */
  function emitResults(results: any, queryTree?: QueryNode) {
    onSearchResults?.({ signal, results, queryTree } as SearchResultEvent)
  }

  function onSubmit() {
    const text = editorView?.state.doc.toString() ?? ''

    if (!text.trim()) {
      searchError = null
      fetchClean()
      return
    }

    try {
      const queryTree: QueryNode | null = parseQuery(text, availableFields)
      searchError = null
      if (!queryTree) {
        fetchClean()
        return
      }

      const durationErr = normalizeDurationValues(queryTree)
      if (durationErr) {
        searchError = durationErr
        return
      }

      const searchCtx = currentSearchContext()
      const searchFn = buildSearchFn(searchCtx, queryTree)
      if (!searchFn) {
        fetchClean()
        return
      }

      searchFn()
        .then(results => {
          emitResults(results, queryTree)
        })
        .catch(err => {
          searchError = 'Search failed: ' + err.message
        })
    } catch (err) {
      searchError = err instanceof Error ? err.message : 'Parse error'
    }
  }

  function openHelp() {
    helpLastFocusedElement =
      document.activeElement instanceof HTMLElement
        ? document.activeElement
        : null
    prevBodyOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    helpDialogElement?.showModal()
    helpDialogOpen = true
    requestAnimationFrame(() => helpDialogElement?.focus())
  }

  function clearSearch() {
    if (editorView) {
      editorView.dispatch({
        changes: { from: 0, to: editorView.state.doc.length, insert: '' },
      })
    }
    searchError = null
    onSubmit()
  }

  function toggleFilter(id: string) {
    activeFilterId = activeFilterId === id ? null : id
  }

  /** Close filter popover when clicking outside. */
  $effect(() => {
    if (!activeFilterId) return
    const handleClick = (e: MouseEvent) => {
      const target = e.target as Node
      if (filterPopoverEl?.contains(target)) return
      const btn = (target as Element).closest?.('[data-filter-id]')
      if (btn) return
      activeFilterId = null
    }
    document.addEventListener('click', handleClick, true)
    return () => document.removeEventListener('click', handleClick, true)
  })

  $effect(() => {
    onSearchError?.(searchError)
  })

  // --- lifecycle ---

  onMount(() => {
    ensureTooltipStyles()

    const mountEl = editorContainer
    if (!mountEl) return

    const state = EditorState.create({
      doc: '',
      extensions: [
        queryLanguageSupport(),
        autocompletion({
          override: [createQueryCompletionSource(() => availableFields)],
          activateOnTyping: true,
          icons: false,
        }),
        createQueryLinter(() => availableFields),
        createQueryKeymap(onSubmit),
        ...queryTheme,
        tooltips({ position: 'fixed' }),
        closeBrackets(),
        history(),
        keymap.of([...defaultKeymap, ...historyKeymap]),
        placeholderCompartment.of(cmPlaceholder(placeholderText)),
        EditorView.lineWrapping,
        ...(variant === 'drawer'
          ? [
              EditorView.theme({
                '&': { height: 'auto', minHeight: '4.75rem' },
                '.cm-scroller': {
                  overflowY: 'auto',
                  maxHeight: 'min(42vh, 13rem)',
                },
              }),
            ]
          : []),
      ],
    })

    editorView = new EditorView({
      state,
      parent: mountEl,
    })

    onReady?.({
      submit: onSubmit,
      clear: clearSearch,
    })
  })

  onDestroy(() => {
    editorView?.destroy()
  })
</script>

{#if variant === 'drawer'}
  <div class="search-editor-wrapper search-editor-wrapper--drawer">
    <div
      class="search-editor-container"
      class:search-editor-container--error={!!searchError}
    >
      {#if headerActions}
        <div class="search-editor__header-actions">
          {@render headerActions()}
        </div>
      {/if}
      <div class="editor-mount" bind:this={editorContainer}></div>
      <div
        class="search-editor__footer-actions"
        class:search-editor__footer-actions--has-error={!!searchError}
      >
        {#if searchError}
          <div class="search-editor-footer__leading min-w-0 flex-1">
            <FieldErrorMessage message={searchError} />
          </div>
        {/if}
        <div class="join join-horizontal shrink-0">
          <button
            type="button"
            class="drawer-editor-btn join-item"
            onclick={openHelp}
            aria-label="Search query help"
            title="Search query help"
          >
            <HelpCircleIcon class="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
          </button>
          <button
            type="button"
            class="drawer-editor-btn join-item"
            onclick={clearSearch}
            aria-label="Clear search"
            title="Clear search"
          >
            <CancelIcon class="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
          </button>
          <button
            type="button"
            class="drawer-editor-btn join-item"
            onclick={onSubmit}
            aria-label="Search"
            title="Search"
          >
            <svg
              class="h-3.5 w-3.5 shrink-0"
              xmlns="http://www.w3.org/2000/svg"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="1.75"
              stroke-linecap="round"
              stroke-linejoin="round"
              aria-hidden="true"
            >
              <path d="m17 17l4 4m-2-10a8 8 0 1 0-16 0a8 8 0 0 0 16 0" />
            </svg>
          </button>
        </div>
      </div>
    </div>
  </div>
{:else}
  <div
    class="search-editor-wrapper"
    class:search-editor-wrapper--in-toolbar={inToolbar}
  >
    <div
      class="search-editor-container"
      class:search-editor-container--error={!!searchError}
    >
      <div class="editor-mount" bind:this={editorContainer}></div>

      <div class="search-actions join">
        <button
          type="button"
          class="btn btn-ghost btn-neutral btn-sm btn-square join-item"
          onclick={openHelp}
          aria-label="Search query help"
          title="Search query help"
        >
          <HelpCircleIcon class="h-4 w-4 shrink-0" aria-hidden="true" />
        </button>
        <button
          type="button"
          class="btn btn-ghost btn-neutral btn-sm btn-square join-item"
          onclick={clearSearch}
          aria-label="Clear search"
          title="Clear search"
        >
          <CancelIcon class="h-4 w-4 shrink-0" aria-hidden="true" />
        </button>
        <button
          type="button"
          class="btn btn-ghost btn-neutral btn-sm btn-square join-item"
          onclick={onSubmit}
          aria-label="Search (Cmd+Enter)"
          title="Search (Cmd+Enter)"
        >
          <svg
            class="h-4 w-4 shrink-0"
            xmlns="http://www.w3.org/2000/svg"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="1.75"
            stroke-linecap="round"
            stroke-linejoin="round"
            aria-hidden="true"
          >
            <path d="m17 17l4 4m-2-10a8 8 0 1 0-16 0a8 8 0 0 0 16 0" />
          </svg>
        </button>
      </div>
    </div>

    {#if activeFilter && !inToolbar}
      <div bind:this={filterPopoverEl} class="search-filter-popover">
        {@render activeFilter.content()}
      </div>
    {/if}
  </div>
{/if}

<!-- Help dialog: rendered OUTSIDE .search-editor-container to avoid clipping -->
<dialog
  bind:this={helpDialogElement}
  class="help-dialog"
  closedby="any"
  tabindex="-1"
>
  <header class="help-dialog-header">
    <h2 class="help-dialog-title">Querying Your {signalLabel}</h2>
    <button
      type="button"
      class="btn btn-ghost btn-sm btn-square shrink-0 text-base-content/70 hover:text-base-content"
      onclick={() => helpDialogElement?.close()}
      aria-label="Close"
    >
      <svg
        class="h-5 w-5"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
        aria-hidden="true"
      >
        <path d="M18 6L6 18M6 6l12 12" />
      </svg>
    </button>
  </header>

  <div class="help-dialog-body">
    <div class="help-query-lines">
      <p>
        Each query follows the pattern <code class="q-field">field</code>
        <code class="q-operator">operator</code>
        <code class="q-value">value</code>
      </p>
      <p>
        Combine filters with <code class="q-logic">AND</code> or
        <code class="q-logic">OR</code>
      </p>
      <p>
        Use parentheses <code class="q-paren">( )</code> to control grouping
      </p>
    </div>

    <h3 class="help-section-heading">Operators</h3>
    <table class="help-op-table">
      <thead>
        <tr>
          <th scope="col">Category</th>
          <th scope="col">Operators</th>
        </tr>
      </thead>
      <tbody>
        <tr>
          <th scope="row">Equality</th>
          <td>
            <code class="help-op-code">=</code>,
            <code class="help-op-code">!=</code>
          </td>
        </tr>
        <tr>
          <th scope="row">Comparison</th>
          <td>
            <code class="help-op-code">&gt;</code>,
            <code class="help-op-code">&lt;</code>,
            <code class="help-op-code">&gt;=</code>,
            <code class="help-op-code">&lt;=</code>
          </td>
        </tr>
        <tr>
          <th scope="row">Text</th>
          <td>
            <code class="help-op-code">CONTAINS</code>,
            <code class="help-op-code">NOT CONTAINS</code>
            <span class="help-op-note">
              <code class="help-op-code">^</code> (starts with)
              <code class="help-op-code">$</code> (ends with)
            </span>
          </td>
        </tr>
        <tr>
          <th scope="row">Pattern</th>
          <td>
            <code class="help-op-code">=~</code>,
            <code class="help-op-code">!~</code>,
            <code class="help-op-code">REGEXP</code>
          </td>
        </tr>
        <tr>
          <th scope="row">List</th>
          <td>
            <code class="help-op-code">IN</code>,
            <code class="help-op-code">NOT IN</code>
          </td>
        </tr>
      </tbody>
    </table>

    <h3 class="help-section-heading">Examples</h3>

    {#if signal === 'traces'}
      <pre class="help-example"><code class="q-field">name</code> <code
          class="q-operator">CONTAINS</code
        > <code class="q-value">http</code></pre>
      <pre class="help-example"><code class="q-field">kind</code> <code
          class="q-operator">=</code
        > <code class="q-value">Server</code> <code class="q-logic">AND</code
        > <code class="q-field">statusCode</code> <code class="q-operator"
          >=</code
        > <code class="q-value">Error</code> <code class="q-logic">AND</code
        > <code class="q-field">name</code> <code class="q-operator"
          >CONTAINS</code
        > <code class="q-value">checkout</code></pre>
    {:else if signal === 'logs'}
      <pre class="help-example"><code class="q-field">severityText</code> <code
          class="q-operator">=</code
        > <code class="q-value">ERROR</code></pre>
      <pre class="help-example"><code class="q-field">severityText</code> <code
          class="q-operator">=</code
        > <code class="q-value">WARN</code> <code class="q-logic">AND</code
        > <code class="q-field">body</code> <code class="q-operator"
          >CONTAINS</code
        > <code class="q-value">disk</code> <code class="q-logic">AND</code
        > <code class="q-field">traceID</code> <code class="q-operator">=</code
        > <code class="q-value">4af9f2c…</code></pre>
    {:else}
      <pre class="help-example"><code class="q-field">name</code> <code
          class="q-operator">CONTAINS</code
        > <code class="q-value">requests</code> <code class="q-logic">AND</code
        > <code class="q-field">type</code> <code class="q-operator">=</code
        > <code class="q-value">Gauge</code></pre>
    {/if}

    <h3 class="help-section-heading">Keyboard shortcuts</h3>
    <ul class="help-shortcut-list">
      <li>
        <kbd class="kbd kbd-sm">⌘</kbd>
        <kbd class="kbd kbd-sm">Enter</kbd>
        <span class="help-shortcut-sep">—</span>
        run query
      </li>
      <li>
        <kbd class="kbd kbd-sm">Ctrl</kbd>
        <kbd class="kbd kbd-sm">Space</kbd>
        <span class="help-shortcut-sep">—</span>
        open autocomplete
      </li>
    </ul>
  </div>
</dialog>

<style lang="postcss">
  @reference "../../../app.css";

  .search-editor-wrapper {
    @apply relative w-full;
    min-height: var(--table-row-h);
    line-height: var(--table-row-h);
  }

  .search-editor-wrapper--drawer {
    @apply min-h-0 flex w-full flex-col gap-0;
    min-height: unset;
    line-height: normal;
  }

  .search-editor-wrapper--in-toolbar {
    @apply w-full shrink-0 overflow-visible px-3 py-1;
    box-sizing: border-box;
  }

  .search-editor-wrapper--drawer .search-editor-container {
    @apply min-h-[4.75rem] rounded-lg pt-2 pb-0;
  }

  .search-editor__header-actions {
    @apply absolute top-1 left-2 right-2 z-10;
  }

  .search-editor-wrapper--drawer .editor-mount :global(.cm-editor) {
    min-height: 4.25rem;
  }

  .search-editor-wrapper--drawer .editor-mount :global(.cm-content) {
    padding-top: 1rem;
    padding-bottom: 1.5rem;
  }

  .search-editor__footer-actions {
    @apply absolute bottom-1 right-2 z-10 flex items-center gap-2;
  }

  .search-editor__footer-actions--has-error {
    @apply items-start;
  }

  /* Shared drawer-footer type scale (hint prose + validation). */
  .search-editor-footer__leading,
  .search-editor-footer__leading :global(.field-error-message) {
    @apply text-xs leading-snug font-normal;
  }

  .search-editor-footer__leading :global(.field-error-message) {
    @apply m-0 text-error;
  }

  .search-editor-footer__hint {
    @apply inline-flex shrink-0 select-none items-center gap-1 tracking-tight text-base-content/55;
  }

  .search-editor-footer__hint-plus {
    @apply text-base-content/45;
  }

  .search-editor-footer__hint-suffix {
    @apply text-base-content/50;
  }

  /* Match kbd chips to the same line box as the footer copy. */
  .search-editor-footer__kbd.kbd {
    font-size: 0.625rem;
    line-height: 1rem;
    min-height: 1rem;
    padding-inline: 0.35rem;
    padding-block: 0;
  }

  .search-editor-footer__actions.join {
    @apply shrink-0;
  }

  .search-editor-container {
    @apply input relative flex w-full items-start px-3;
    height: fit-content;
    min-height: var(--table-row-h);
    border-color: var(--color-base-300);
  }

  .search-editor-container:focus-within {
    outline: 1px solid var(--color-primary);
    outline-offset: 1px;
  }

  .search-editor-container--error {
    border-color: var(--color-error);
  }

  .search-editor-container--error:focus-within {
    outline-color: var(--color-error);
  }

  .editor-mount {
    @apply flex-1 min-w-0;
  }

  .editor-mount :global(.cm-editor) {
    @apply bg-transparent;
  }

  .editor-mount :global(.cm-focused) {
    outline: none;
  }

  .editor-mount :global(.cm-content) {
    padding-right: 4rem;
    padding-bottom: 0.25rem;
  }

  .search-actions {
    @apply absolute right-0 bottom-0 z-10 flex items-center;
    --join-radius: 0;
  }

  .search-actions :global(.join-item:first-child) {
    border-top-left-radius: var(--radius-field);
  }

  .search-actions :global(.join-item:last-child) {
    border-bottom-right-radius: var(--radius-field);
  }

  .search-filter-popover {
    @apply border-t border-base-300/70 bg-base-100/80 backdrop-blur-sm;
    @apply px-3 py-2;
  }

  .help-dialog {
    position: fixed;
    top: 50%;
    left: 50%;
    translate: -50% -50%;
    margin: 0;
    box-sizing: border-box;
    width: min(32rem, calc(100vw - 2rem));
    max-height: min(85vh, calc(100vh - 2rem));
    overflow: hidden;

    @apply rounded-lg border border-base-300 bg-base-100 p-0 text-base-content shadow-xl;
    overscroll-behavior: contain;
  }

  .help-dialog[open] {
    display: flex;
    flex-direction: column;
  }

  .help-dialog::backdrop {
    background-color: rgba(0, 0, 0, 0.2);
    backdrop-filter: blur(6px);
  }

  .help-dialog-header {
    @apply flex shrink-0 items-center justify-between gap-3 border-b border-base-300 px-4 py-3;
  }

  .help-dialog-title {
    @apply min-w-0 flex-1 text-lg font-semibold leading-snug tracking-tight;
  }

  .help-dialog-body {
    @apply overflow-y-auto px-4 pb-4 pt-3;
    min-height: 0;
    overscroll-behavior: contain;
  }

  .help-section-heading {
    @apply mt-4 text-sm font-semibold uppercase tracking-wide text-base-content/70;
  }

  .help-query-lines {
    @apply mt-1 space-y-1 text-sm text-base-content/80;
  }

  .help-op-table {
    @apply mt-2 w-full border-collapse text-left text-sm text-base-content/80;
  }

  .help-op-table thead th {
    @apply pb-2 pt-1 text-xs font-semibold uppercase tracking-wide text-base-content/70;
  }

  .help-op-table tbody th {
    @apply w-[5.5rem] max-w-[7rem] py-2 pr-3 align-top text-left text-xs font-medium text-base-content/80;
  }

  .help-op-table tbody td {
    @apply py-2 align-top;
  }

  .help-op-code {
    @apply rounded bg-base-200/80 px-1 py-0.5 font-mono text-xs text-base-content;
  }

  .help-op-note {
    @apply mt-1 block text-xs text-base-content/75;
  }

  .help-op-note .help-op-code {
    @apply mx-0.5;
  }

  .help-shortcut-list {
    @apply mt-2 space-y-2 text-sm text-base-content/80;
  }

  .help-shortcut-list li {
    @apply flex flex-wrap items-center gap-1.5;
  }

  .help-shortcut-sep {
    @apply mx-0.5 text-base-content/50;
  }

  .help-example {
    @apply mt-2 overflow-x-auto rounded-md bg-base-200 p-3 font-mono text-xs leading-relaxed text-base-content;
  }

  .help-example :global(code) {
    @apply whitespace-pre-wrap break-all bg-transparent p-0;
  }

  /* Match CodeMirror query token colors (see lang/theme.ts + app.css --color-*) */
  .q-field {
    color: var(--color-accent);
  }

  .q-operator {
    color: var(--color-subtle);
  }

  .q-logic {
    color: var(--color-primary);
    font-weight: 600;
  }

  .q-value {
    color: var(--color-warning);
  }

  .q-paren {
    color: var(--color-subtle);
  }
</style>

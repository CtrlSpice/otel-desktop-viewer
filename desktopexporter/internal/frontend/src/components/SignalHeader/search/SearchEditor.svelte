<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import { EditorView, placeholder as cmPlaceholder } from '@codemirror/view'
  import { EditorState } from '@codemirror/state'
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
  import { getTimeContext, selectionToQueryRangeMs } from '@/contexts/time-context.svelte'
  import type { TimeContext } from '@/contexts/time-context.svelte'
  import type { SearchResultEvent } from '@/types/api-types'
  import { queryLanguageSupport } from './lang/query-language'
  import { createQueryCompletionSource } from './lang/completions'
  import { createQueryLinter } from './lang/linter'
  import { queryTheme } from './lang/theme'
  import { createQueryKeymap } from './lang/keymap'

  // --- types ---
  type SearchEditorProps =
    | {
        signal: 'traces' | 'metrics' | 'logs'
        view: 'list'
        onSearchResults?: (event: SearchResultEvent) => void
      }
    | {
        signal: 'traces'
        view: 'detail'
        traceID: string
        onSearchResults?: (event: SearchResultEvent) => void
      }
    | {
        signal: 'metrics'
        view: 'detail'
        onSearchResults?: (event: SearchResultEvent) => void
      }

  // --- helpers ---

  /** Build the API call for a signal+view, or null if unsupported. */
  function buildSearchFn(
    sig: SearchEditorProps['signal'],
    v: string,
    startTime: number,
    endTime: number,
    queryTree: QueryNode
  ): (() => Promise<any>) | null {
    if (sig === 'traces') {
      if (v === 'list') {
        return () => telemetryAPI.searchTraces(startTime, endTime, queryTree)
      }
      if (v === 'detail' && traceID) {
        return () => telemetryAPI.searchTraceSpans(traceID, queryTree)
      }
      return null
    }
    if (sig === 'logs') {
      return () => telemetryAPI.searchLogs(startTime, endTime, queryTree)
    }
    return () => telemetryAPI.getMetrics(startTime, endTime, queryTree)
  }

  /** Label after "Search " in the editor placeholder (list vs detail). */
  function searchPlaceholderSubject(
    sig: SearchEditorProps['signal'],
    v: SearchEditorProps['view']
  ): string {
    if (v === 'detail' && sig === 'traces') return 'Spans'
    if (v === 'detail' && sig === 'metrics') return 'Data Points'
    return sig.charAt(0).toUpperCase() + sig.slice(1)
  }

  /** Detect pasted trace/span IDs and wrap with the field name. */
  function idReplacementForPaste(text: string): string | null {
    const trimmed = text.trim()
    if (/^[a-f0-9]{32}$/i.test(trimmed)) return `traceID = ${trimmed}`
    if (/^[a-f0-9]{16}$/i.test(trimmed)) return `spanID = ${trimmed}`
    return null
  }

  // --- context ---
  let { signal, view, onSearchResults, ...rest }: SearchEditorProps = $props()
  let traceID = $derived('traceID' in rest ? rest.traceID : undefined)

  let timeContext: TimeContext | null = null
  try {
    timeContext = getTimeContext()
  } catch {
    console.warn('SearchEditor: time context not available')
  }

  // --- state: editor ---
  let editorContainer: HTMLDivElement
  let editorView: EditorView | null = null
  let searchError = $state<string | null>(null)

  // --- state: help dialog ---
  let helpDialogElement = $state<HTMLDialogElement | null>(null)
  let helpDialogOpen = $state(false)
  let helpLastFocusedElement = $state<HTMLElement | null>(null)
  let prevBodyOverflow = $state<string | null>(null)
  const supportsClosedBy = 'closedBy' in HTMLDialogElement.prototype

  // --- state: available fields (static + dynamic attributes) ---
  let availableFields = $state<FieldDefinition[]>([])

  // --- derived ---
  let signalLabel = $derived(signal.charAt(0).toUpperCase() + signal.slice(1))
  let staticFieldsList = $derived([...getStaticFieldsForSearch(signal)])
  let placeholderText = $derived(
    `Search ${searchPlaceholderSubject(signal, view)}… (Cmd+Enter to submit)`
  )

  // --- effects ---

  /** Debounced fetch of dynamic attributes when time selection or signal changes. */
  $effect(() => {
    const base = [...staticFieldsList]
    availableFields = base
    const tc = timeContext
    if (!tc) return

    let cancelled = false
    const t = window.setTimeout(async () => {
      try {
        const dynamicAttrs = await getDynamicAttributes(
          tc.selection.start,
          tc.selection.end,
          signal
        )
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

  function onSubmit() {
    const text = editorView?.state.doc.toString() ?? ''
    if (!text.trim()) return

    try {
      const queryTree: QueryNode | null = parseQuery(text, availableFields)
      searchError = null
      if (!queryTree) return

      const { start: startTime, end: endTime } = timeContext
        ? selectionToQueryRangeMs(timeContext.selection, Date.now())
        : { start: 0, end: Date.now() }

      const searchFn = buildSearchFn(signal, view, startTime, endTime, queryTree)
      if (!searchFn) {
        console.warn('Detail view search not yet implemented')
        return
      }

      searchFn()
        .then(results => {
          onSearchResults?.({ signal, view, results } as SearchResultEvent)
        })
        .catch(err => {
          searchError = 'Search failed: ' + err.message
        })
    } catch (error) {
      searchError = error instanceof Error ? error.message : 'Parse error'
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

  // --- CodeMirror extensions (static; reference closures over mutable state) ---

  const pasteHandler = EditorView.inputHandler.of((view, from, to, text) => {
    const replacement = idReplacementForPaste(text)
    if (replacement) {
      view.dispatch({
        changes: { from, to, insert: replacement },
        selection: { anchor: from + replacement.length },
      })
      return true
    }
    return false
  })

  // --- lifecycle ---

  onMount(() => {
    const state = EditorState.create({
      doc: '',
      extensions: [
        queryLanguageSupport(),
        autocompletion({
          override: [createQueryCompletionSource(() => availableFields)],
          activateOnTyping: true,
        }),
        createQueryLinter(() => availableFields),
        createQueryKeymap(onSubmit),
        ...queryTheme,
        closeBrackets(),
        history(),
        keymap.of([...defaultKeymap, ...historyKeymap]),
        cmPlaceholder(placeholderText),
        pasteHandler,
        EditorView.lineWrapping,
      ],
    })

    editorView = new EditorView({
      state,
      parent: editorContainer,
    })
  })

  onDestroy(() => {
    editorView?.destroy()
  })
</script>

<div class="search-editor-wrapper">
  <div class="search-editor-container">
    <!-- CodeMirror Editor -->
    <div class="editor-mount" bind:this={editorContainer}></div>

    <!-- Actions pinned top-right while editor grows -->
    <div class="search-actions">
      <button
        type="button"
        class="help-button"
        onclick={openHelp}
        aria-label="Search query help"
        title="Search query help"
      >
        <svg class="h-4 w-4" viewBox="0 0 24 24" aria-hidden="true">
          <g fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round">
            <circle cx="12" cy="12" r="10" stroke-width="1.5"></circle>
            <path stroke-width="1.5" d="M9.5 9.5a2.5 2.5 0 1 1 3.912 2.064C12.728 12.032 12 12.672 12 13.5"></path>
            <path stroke-width="1.8" d="M12 17h.009"></path>
          </g>
        </svg>
      </button>

      <button
        class="search-button"
        onclick={onSubmit}
        aria-label="Search (Cmd+Enter)"
        title="Search (Cmd+Enter)"
      >
        <svg
          class="search-button-icon"
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

  {#if searchError}
    <div class="error-message">
      {searchError}
    </div>
  {/if}
</div>

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
      class="btn btn-ghost btn-square btn-sm shrink-0 text-base-content/70 hover:text-base-content"
      onclick={() => helpDialogElement?.close()}
      aria-label="Close"
    >
      <svg class="h-5 w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
        <path d="M18 6L6 18M6 6l12 12" />
      </svg>
    </button>
  </header>

  <div class="help-dialog-body">
    <div class="help-query-lines">
      <p>Each query follows the pattern <code class="q-field">field</code> <code class="q-operator">operator</code> <code class="q-value">value</code></p>
      <p>Combine filters with <code class="q-logic">AND</code> or <code class="q-logic">OR</code></p>
      <p>Use parentheses <code class="q-paren">( )</code> to control grouping</p>
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
            <code class="help-op-code">=</code>, <code class="help-op-code">!=</code>
          </td>
        </tr>
        <tr>
          <th scope="row">Comparison</th>
          <td>
            <code class="help-op-code">&gt;</code>, <code class="help-op-code">&lt;</code>,
            <code class="help-op-code">&gt;=</code>, <code class="help-op-code">&lt;=</code>
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
            <code class="help-op-code">=~</code>, <code class="help-op-code">!~</code>,
            <code class="help-op-code">REGEXP</code>
          </td>
        </tr>
        <tr>
          <th scope="row">List</th>
          <td>
            <code class="help-op-code">IN</code>, <code class="help-op-code">NOT IN</code>
          </td>
        </tr>
      </tbody>
    </table>

    <h3 class="help-section-heading">Examples</h3>

    {#if signal === 'traces'}
      <pre class="help-example"><code class="q-field">name</code> <code class="q-operator">CONTAINS</code> <code class="q-value">http</code></pre>
      <pre class="help-example"><code class="q-field">kind</code> <code class="q-operator">=</code> <code class="q-value">Server</code> <code class="q-logic">AND</code> <code class="q-field">statusCode</code> <code class="q-operator">=</code> <code class="q-value">Error</code> <code class="q-logic">AND</code> <code class="q-field">name</code> <code class="q-operator">CONTAINS</code> <code class="q-value">checkout</code></pre>
    {:else if signal === 'logs'}
      <pre class="help-example"><code class="q-field">severityText</code> <code class="q-operator">=</code> <code class="q-value">ERROR</code></pre>
      <pre class="help-example"><code class="q-field">severityText</code> <code class="q-operator">=</code> <code class="q-value">WARN</code> <code class="q-logic">AND</code> <code class="q-field">body</code> <code class="q-operator">CONTAINS</code> <code class="q-value">disk</code> <code class="q-logic">AND</code> <code class="q-field">traceID</code> <code class="q-operator">=</code> <code class="q-value">4af9f2c…</code></pre>
    {:else}
      <pre class="help-example"><code class="q-field">name</code> <code class="q-operator">CONTAINS</code> <code class="q-value">requests</code> <code class="q-logic">AND</code> <code class="q-field">type</code> <code class="q-operator">=</code> <code class="q-value">Gauge</code></pre>
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

    <h3 class="help-section-heading">Pasting IDs</h3>
    <p class="mt-1 text-sm text-base-content/80">
      Paste a 32-character trace ID or 16-character span ID and it auto-fills as
      <code class="text-xs">traceID = …</code> or <code class="text-xs">spanID = …</code>.
    </p>
  </div>
</dialog>

<style lang="postcss">
  @reference "../../../app.css";
  .search-editor-wrapper {
    @apply relative;
  }

  .search-editor-container {
    @apply input;
    @apply relative flex min-h-10 w-full items-start px-3 py-1;
    min-height: 2.5rem;
    height: auto;
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
    /* Keep text clear of top-right action buttons */
    padding-right: 5.5rem;
  }

  .search-actions {
    @apply absolute right-1.5 top-1.5 z-10 flex items-center gap-1;
  }

  .search-button {
    @apply btn btn-primary btn-square;
    @apply h-8 min-h-8 w-8 p-0;
  }

  .search-button-icon {
    @apply block h-4 w-4;
  }

  .error-message {
    @apply absolute top-full left-0 right-0;
    @apply mt-1 px-3 py-2;
    @apply bg-error/10 text-error;
    @apply text-xs rounded-md;
    @apply border border-error/20;
  }

  .help-button {
    @apply btn btn-ghost btn-square;
    @apply h-8 min-h-8 w-8 p-0 text-base-content/70 hover:text-base-content;
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

  .help-signal-heading {
    @apply mt-4 text-sm font-semibold text-base-content;
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

  .help-signal-heading + .help-example {
    @apply mt-1;
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

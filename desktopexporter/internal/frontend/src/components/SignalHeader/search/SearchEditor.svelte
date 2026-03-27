<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { EditorView, placeholder as cmPlaceholder } from '@codemirror/view';
  import { EditorState } from '@codemirror/state';
  import { autocompletion, closeBrackets } from '@codemirror/autocomplete';
  import { history, defaultKeymap, historyKeymap } from '@codemirror/commands';
  import { keymap } from '@codemirror/view';
  import {
    getStaticFieldsForSearch,
    getDynamicAttributes,
    type FieldDefinition,
  } from '@/constants/fields';
  import { parseQuery } from './queryParser';
  import type { QueryNode } from './queryTree';
  import { telemetryAPI } from '@/services/telemetry-service';
  import { getTimeContext } from '@/contexts/time-context.svelte';
  import type { SearchResultEvent } from '@/types/api-types';
  import { queryLanguageSupport } from './lang/query-language';
  import { createQueryCompletionSource } from './lang/completions';
  import { createQueryLinter } from './lang/linter';
  import { queryTheme } from './lang/theme';
  import { createQueryKeymap } from './lang/keymap';
  type SearchEditorProps =
    | {
        signal: 'logs';
        view: 'list';
        onSearchResults?: (event: SearchResultEvent) => void;
      }
    | {
        signal: 'traces' | 'metrics';
        view: 'list' | 'detail';
        onSearchResults?: (event: SearchResultEvent) => void;
      };

  let {
    signal,
    view,
    onSearchResults,
  }: SearchEditorProps = $props();
  let signalLabel = $derived(signal.charAt(0).toUpperCase() + signal.slice(1));

  let timeContext: any = null;
  try {
    timeContext = getTimeContext();
  } catch (error) {
    console.warn('Could not get time context during initialization:', error);
  }

  let staticFieldsList = $derived([...getStaticFieldsForSearch(signal)]);
  let availableFields = $state<FieldDefinition[]>([]);

  $effect(() => {
    const base = [...staticFieldsList];
    availableFields = base;
    const tc = timeContext;
    if (!tc) return;

    let cancelled = false;
    const t = window.setTimeout(async () => {
      try {
        const dynamicAttrs = await getDynamicAttributes(
          tc.selection.start,
          tc.selection.end,
          signal
        );
        if (cancelled) return;
        availableFields = [...base, ...dynamicAttrs];
      } catch (error) {
        console.warn('Failed to load dynamic attributes:', error);
      }
    }, 250);

    return () => {
      cancelled = true;
      window.clearTimeout(t);
    };
  });

  let editorContainer: HTMLDivElement;
  let editorView: EditorView | null = null;
  let searchError = $state<string | null>(null);

  let placeholderText = $derived(`Search ${signalLabel}... (Cmd+Enter to submit)`);

  function onSubmit() {
    const text = editorView?.state.doc.toString() ?? '';
    if (!text.trim()) return;

    try {
      const queryTree: QueryNode | null = parseQuery(text, availableFields);
      searchError = null;

      if (!queryTree) return;

      let startTime = 0;
      let endTime = Date.now();

      if (timeContext) {
        if (timeContext.selection.type === 'preset') {
          const duration = timeContext.selection.end - timeContext.selection.start;
          startTime = endTime - duration;
        } else {
          startTime = timeContext.selection.start;
          endTime = timeContext.selection.end;
        }
      }

      const searchBySignal = {
        traces: {
          list: () => telemetryAPI.searchTraces(startTime, endTime, queryTree),
          detail: null,
        },
        logs: {
          list: () => telemetryAPI.searchLogs(startTime, endTime, queryTree),
        },
        metrics: {
          list: () => telemetryAPI.getMetrics(startTime, endTime, queryTree),
          detail: () => telemetryAPI.getMetrics(startTime, endTime, queryTree),
        },
      } as const;

      const emitResultsBySignal = {
        traces: (results: any) =>
          onSearchResults?.({ signal: 'traces', view: view as 'list' | 'detail', results }),
        logs: (results: any) =>
          onSearchResults?.({ signal: 'logs', view: 'list', results }),
        metrics: (results: any) =>
          onSearchResults?.({ signal: 'metrics', view, results }),
      } as const;

      const searchFn: (() => Promise<any>) | null =
        signal === 'traces'
          ? searchBySignal.traces[view]
          : signal === 'logs'
            ? searchBySignal.logs[view as 'list']
            : searchBySignal.metrics[view];

      if (!searchFn) {
        console.warn('Detail view search not yet implemented');
        return;
      }

      searchFn()
        .then(results => {
          emitResultsBySignal[signal](results);
        })
        .catch(error => {
          searchError = 'Search failed: ' + error.message;
        });
    } catch (error) {
      searchError = error instanceof Error ? error.message : 'Parse error';
    }
  }

  // Paste handler: detect trace/span IDs and wrap with field name
  const pasteHandler = EditorView.inputHandler.of((view, from, to, text) => {
    const trimmed = text.trim();
    const traceIdPattern = /^[a-f0-9]{32}$/i;
    const spanIdPattern = /^[a-f0-9]{16}$/i;

    let replacement: string | null = null;
    if (traceIdPattern.test(trimmed)) {
      replacement = `traceID = ${trimmed}`;
    } else if (spanIdPattern.test(trimmed)) {
      replacement = `spanID = ${trimmed}`;
    }

    if (replacement) {
      view.dispatch({
        changes: { from, to, insert: replacement },
        selection: { anchor: from + replacement.length },
      });
      return true;
    }

    return false;
  });

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
    });

    editorView = new EditorView({
      state,
      parent: editorContainer,
    });
  });

  onDestroy(() => {
    editorView?.destroy();
  });

  // Help dialog
  let helpDialogElement = $state<HTMLDialogElement | null>(null);
  let helpDialogOpen = $state(false);
  let helpLastFocusedElement = $state<HTMLElement | null>(null);
  let prevBodyOverflow = $state<string | null>(null);

  const supportsClosedBy = 'closedBy' in HTMLDialogElement.prototype;

  function openHelp() {
    helpLastFocusedElement =
      (document.activeElement instanceof HTMLElement
        ? document.activeElement
        : null);
    prevBodyOverflow = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    helpDialogElement?.showModal();
    helpDialogOpen = true;
    requestAnimationFrame(() => helpDialogElement?.focus());
  }

  $effect(() => {
    if (helpDialogElement) {
      const handleClose = () => {
        helpDialogOpen = helpDialogElement?.open ?? false;
        if (prevBodyOverflow !== null) {
          document.body.style.overflow = prevBodyOverflow;
          prevBodyOverflow = null;
        }
        if (helpLastFocusedElement?.isConnected) {
          helpLastFocusedElement.focus();
        }
        helpLastFocusedElement = null;
      };

      const handleCancel = () => {
        helpDialogOpen = false;
      };

      const handleClickOutside = (event: MouseEvent) => {
        if (!supportsClosedBy && helpDialogElement) {
          const rect = helpDialogElement.getBoundingClientRect();
          const isInDialog =
            rect.top <= event.clientY &&
            event.clientY <= rect.top + rect.height &&
            rect.left <= event.clientX &&
            event.clientX <= rect.left + rect.width;

          if (!isInDialog) {
            helpDialogElement.close();
          }
        }
      };

      helpDialogElement.addEventListener('close', handleClose);
      helpDialogElement.addEventListener('cancel', handleCancel);

      if (!supportsClosedBy) {
        helpDialogElement.addEventListener('click', handleClickOutside);
      }

      helpDialogOpen = helpDialogElement.open;

      return () => {
        helpDialogElement?.removeEventListener('close', handleClose);
        helpDialogElement?.removeEventListener('cancel', handleCancel);
        if (!supportsClosedBy) {
          helpDialogElement?.removeEventListener('click', handleClickOutside);
        }
      };
    }
  });
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
  .search-editor-wrapper {
    @apply relative;
  }

  .search-editor-container {
    @apply input input-bordered;
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
    @apply border-b border-base-300 pb-2 pt-1 text-xs font-semibold uppercase tracking-wide text-base-content/70;
  }

  .help-op-table tbody th {
    @apply w-[5.5rem] max-w-[7rem] border-b border-base-300 py-2 pr-3 align-top text-left text-xs font-medium text-base-content/80;
  }

  .help-op-table tbody td {
    @apply border-b border-base-300 py-2 align-top;
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

  /* Match CodeMirror query token colors via shared CSS vars */
  .q-field {
    color: var(--query-foam);
  }

  .q-operator {
    color: var(--query-subtle);
  }

  .q-logic {
    color: var(--query-iris);
    font-weight: 600;
  }

  .q-value {
    color: var(--query-gold);
  }

  .q-paren {
    color: var(--query-subtle);
  }
</style>

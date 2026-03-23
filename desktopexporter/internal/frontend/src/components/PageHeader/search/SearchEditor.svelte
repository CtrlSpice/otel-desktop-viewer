<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { EditorView, placeholder as cmPlaceholder } from '@codemirror/view';
  import { EditorState } from '@codemirror/state';
  import { autocompletion, closeBrackets } from '@codemirror/autocomplete';
  import { history, defaultKeymap, historyKeymap } from '@codemirror/commands';
  import { keymap } from '@codemirror/view';
  import {
    getFieldsBySignal,
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
        placeholder?: string;
        onSearchResults?: (event: SearchResultEvent) => void;
      }
    | {
        signal: 'traces' | 'metrics';
        view: 'list' | 'detail';
        placeholder?: string;
        onSearchResults?: (event: SearchResultEvent) => void;
      };

  let {
    signal,
    view,
    placeholder,
    onSearchResults,
  }: SearchEditorProps = $props();

  let timeContext: any = null;
  try {
    timeContext = getTimeContext();
  } catch (error) {
    console.warn('Could not get time context during initialization:', error);
  }

  const staticFields = [...getFieldsBySignal(signal)];
  let availableFields = $state<FieldDefinition[]>([...staticFields]);

  async function loadDynamicAttributes() {
    if (timeContext) {
      let dynamicAttrs = await getDynamicAttributes(
        timeContext.selection.start,
        timeContext.selection.end,
        signal
      );
      availableFields = [...staticFields, ...dynamicAttrs];
    }
  }

  loadDynamicAttributes();

  let editorContainer: HTMLDivElement;
  let editorView: EditorView | null = null;
  let searchError = $state<string | null>(null);

  function getPlaceholderText(): string {
    if (placeholder) return placeholder;
    if (signal) return `Search ${signal}... (Cmd+Enter to submit)`;
    return 'Search... (Cmd+Enter to submit)';
  }

  function onSubmit() {
    const text = editorView?.state.doc.toString() ?? '';
    if (!text.trim()) return;

    try {
      const queryTree: QueryNode | null = parseQuery(text, availableFields);
      searchError = null;

      if (queryTree) {
        let startTime = 0;
        let endTime = Date.now();

        if (timeContext) {
          if (timeContext.selection.type === 'preset') {
            const duration =
              timeContext.selection.end - timeContext.selection.start;
            startTime = endTime - duration;
          } else {
            startTime = timeContext.selection.start;
            endTime = timeContext.selection.end;
          }
        }

        let searchPromise: Promise<any>;

        switch (signal) {
          case 'traces':
            if (view === 'list') {
              searchPromise = telemetryAPI.searchTraces(
                startTime,
                endTime,
                queryTree
              );
            } else {
              console.warn('Detail view search not yet implemented');
              return;
            }
            break;
          case 'logs':
            console.warn('Logs search not yet implemented');
            return;
          case 'metrics':
            console.warn('Metrics search not yet implemented');
            return;
          default:
            throw new Error(`Unknown signal type: ${signal}`);
        }

        searchPromise
          .then(results => {
            let resultEvent: SearchResultEvent = { signal, view, results };
            onSearchResults?.(resultEvent);
          })
          .catch(error => {
            searchError = 'Search failed: ' + error.message;
          });
      }
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
        cmPlaceholder(getPlaceholderText()),
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
</script>

<div class="search-editor-wrapper">
  <div class="search-editor-container">
    <!-- Search Icon -->
    <svg
      class="search-icon"
      xmlns="http://www.w3.org/2000/svg"
      width="16"
      height="16"
      viewBox="0 0 24 24"
    >
      <path d="m17 17l4 4m-2-10a8 8 0 1 0-16 0a8 8 0 0 0 16 0" />
    </svg>

    <!-- CodeMirror Editor -->
    <div class="editor-mount" bind:this={editorContainer}></div>

    <!-- Submit Button -->
    <button
      class="search-button"
      onclick={onSubmit}
      aria-label="Search (Cmd+Enter)"
      title="Search (Cmd+Enter)"
    >
      <svg
        xmlns="http://www.w3.org/2000/svg"
        width="16"
        height="16"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-linecap="round"
        stroke-linejoin="round"
      >
        <path d="m9 18l6-6l-6-6" />
      </svg>
    </button>
  </div>

  {#if searchError}
    <div class="error-message">
      {searchError}
    </div>
  {/if}
</div>

<style lang="postcss">
  .search-editor-wrapper {
    @apply relative;
  }

  .search-editor-container {
    @apply flex items-start gap-2;
    @apply input input-bordered;
    @apply w-full px-2 py-1;
    min-height: 2.5rem;
    height: auto;
  }

  .search-icon {
    @apply w-4 h-4 flex-shrink-0 text-base-content/60;
    margin-top: 10px;
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

  .search-button {
    @apply flex items-center justify-center;
    @apply w-6 h-6;
    @apply text-base-content/60;
    @apply transition-colors;
    @apply hover:text-primary;
    margin-top: 8px;
  }

  .error-message {
    @apply absolute top-full left-0 right-0;
    @apply mt-1 px-3 py-2;
    @apply bg-error/10 text-error;
    @apply text-xs rounded-md;
    @apply border border-error/20;
  }
</style>

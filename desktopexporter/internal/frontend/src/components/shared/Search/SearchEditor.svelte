<script lang="ts" generics="TSortColumn extends string">
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
  import { getStaticFieldsForSearch } from '@/constants/fields'
  import { getDynamicAttributes } from '@/services/search-field-service'
  import type {
    FieldDefinition,
    SearchSignal,
    ParsedSearchRequest,
  } from '@/search/model'
  import { parseSearchRequest } from './queryParser'
  import { telemetryAPI } from '@/services/telemetry-service'
  import {
    getTimeContext,
    selectionToQueryRangeMs,
    selectionToQueryRangeNs,
  } from '@/contexts/time-context.svelte'
  import type { SearchResultEvent } from '@/types/api-types'
  import { runSearch, type SearchContext } from './search-dispatch'
  import {
    beginListUpdate,
    cancelPendingListUpdates,
  } from '@/components/shared/utils/list-update-seq'
  import type { FilterDescriptor } from '@/components/shared/Toolbar/filter-types'
  import { queryLanguageSupport } from './codemirror/query-language'
  import { createQueryCompletionSource } from './codemirror/completions'
  import { createFieldValueCache } from '@/components/shared/Search/codemirror/field-value-cache'
  import { createFieldValueSource } from '@/components/shared/Search/codemirror/field-value-completions'
  import { storedKindForField } from './attribute-field-reference'
  import { createQueryLinter } from './codemirror/linter'
  import { queryTheme } from './codemirror/theme'
  import { createQueryKeymap } from './codemirror/keymap'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { Cancel01Icon } from '@hugeicons/core-free-icons'
  import { QuestionMarkIcon } from '@/icons'
  import FieldErrorMessage from '@/components/shared/FieldErrorMessage.svelte'
  import SearchHelpDialog from './SearchHelpDialog.svelte'
  import { attributeTokens } from './codemirror/attribute-tokens'
  import { multilineQueryPaste } from './codemirror/paste'
  import { navigateCurrentRoute, readRoute, subscribeToRoute } from '@/route'

  import type { SearchDefinition, SearchEditorAPI } from './search-editor-api'
  import type { SortDirection } from '@/contexts/signal-list-page.svelte'

  type SearchEditorProps = {
    sortValue: TSortColumn
    sortDirection: SortDirection
    onSearchResults?: (event: SearchResultEvent) => void
    /** Toolbar layout: panel chrome on the search wrapper only (not the action row). */
    inToolbar?: boolean
    /** 'drawer' moves action buttons to a footer row below the editor. */
    variant?: 'default' | 'drawer'
    /** Called once after mount with imperative handles. */
    onReady?: (api: SearchEditorAPI) => void
    /** Called whenever the search error state changes. */
    onSearchError?: (error: string | null) => void
    onQueryChange?: (text: string, fields: FieldDefinition[]) => void
    onQuerySubmit?: (text: string) => void
    /** A paged caller owns fetching after the editor validates the query. */
    onSearchRequest?: (request: ParsedSearchRequest | null) => void
    /** Actions pinned top-right inside the editor container (drawer variant only). */
    headerActions?: Snippet
  } & (
    | { signal: SearchSignal; definition?: never }
    | {
        signal?: never
        definition: SearchDefinition
        onSearchRequest: (request: ParsedSearchRequest | null) => void
      }
  )

  let {
    signal,
    definition,
    sortValue,
    sortDirection,
    onSearchResults,
    inToolbar = false,
    variant = 'default',
    onReady,
    onSearchError,
    onQueryChange,
    onQuerySubmit,
    onSearchRequest,
    headerActions,
    filters = [],
  }: SearchEditorProps & { filters?: FilterDescriptor[] } = $props()

  const isMac =
    typeof navigator !== 'undefined' &&
    /Mac|iPhone|iPad/.test(navigator.userAgent)
  const modKey = isMac ? '⌘' : 'Ctrl'

  const timeContext = getTimeContext()

  let editorContainer = $state<HTMLDivElement | null>(null)
  let editorView: EditorView | null = null
  let searchError = $state<string | null>(null)
  let alive = true
  const placeholderCompartment = new Compartment()

  let activeFilterID = $state<string | null>(null)
  let filterPopoverEl = $state<HTMLDivElement | null>(null)

  let activeFilter = $derived(
    activeFilterID ? (filters.find(f => f.id === activeFilterID) ?? null) : null
  )

  let helpDialogElement = $state<HTMLDialogElement | null>(null)
  let helpDialogOpen = $state(false)
  let helpLastFocusedElement = $state<HTMLElement | null>(null)
  let prevBodyOverflow = $state<string | null>(null)
  const supportsClosedBy = 'closedBy' in HTMLDialogElement.prototype

  let availableFields = $state<FieldDefinition[]>([])
  let queryText = $state('')
  let definitionsLoaded = false
  let pendingRestoration: string | undefined

  $effect(() => onQueryChange?.(queryText, availableFields))

  let staticFieldsList = $derived(
    definition?.fields ?? (signal ? [...getStaticFieldsForSearch(signal)] : [])
  )
  let placeholderText = $derived(`search ${definition?.label ?? signal}...`)

  $effect(() => {
    editorView?.dispatch({
      effects: placeholderCompartment.reconfigure(
        cmPlaceholder(placeholderText)
      ),
    })
  })

  /** Fetch dynamic attributes from the shared dictionary. */
  $effect(() => {
    const base = [...staticFieldsList]
    const requestedSignal = signal
    definitionsLoaded = false
    availableFields = base

    if (!requestedSignal) {
      definitionsLoaded = true
      return
    }

    let cancelled = false
    const t = window.setTimeout(async () => {
      try {
        void selectionToQueryRangeMs(timeContext.selection, Date.now())
        const dynamicAttrs = await getDynamicAttributes(requestedSignal)
        if (cancelled) return
        availableFields = [...base, ...dynamicAttrs]
      } catch (error) {
        console.warn('Failed to load dynamic attributes:', error)
      } finally {
        if (!cancelled) {
          definitionsLoaded = true
          if (
            pendingRestoration !== undefined &&
            pendingRestoration === editorView?.state.doc.toString()
          )
            onSubmit()
        }
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

  /** Build a SearchContext from the current component state. */
  function currentSearchContext(signal: SearchSignal): SearchContext {
    const { startTime, endTime } = selectionToQueryRangeNs(
      timeContext.selection,
      Date.now()
    )

    return { signal, startTime, endTime }
  }

  /** Fetch without any search filter and deliver via onSearchResults. */
  function fetchClean(updateSeq: number) {
    if (!signal) return
    const ctx = currentSearchContext(signal)
    executeSearch(runSearch(ctx, updateSeq))
  }

  function executeSearch(search: Promise<SearchResultEvent>) {
    search
      .then(event => {
        if (!alive) return
        onSearchResults?.(event)
      })
      .catch(err => {
        if (!alive) return
        searchError = 'Search failed: ' + err.message
      })
  }

  let routeQuery = ''
  function rememberQuery(text: string) {
    const route = readRoute()
    const next = text.trim() ? text : ''
    routeQuery = next
    if ((route.query.q ?? '') !== next)
      navigateCurrentRoute({ ...route.query, q: next || undefined }, 'replace')
  }

  function onSubmit() {
    pendingRestoration = undefined
    const text = editorView?.state.doc.toString() ?? ''
    // Initial URL submission can precede the reactive change notification.
    onQueryChange?.(text, availableFields)

    if (!text.trim()) {
      searchError = null
      rememberQuery(text)
      onQuerySubmit?.(text)
      if (onSearchRequest) {
        onSearchRequest(null)
        return
      }
      if (signal) fetchClean(beginListUpdate(signal))
      return
    }

    try {
      const request = parseSearchRequest(text, availableFields, signal)
      searchError = null
      rememberQuery(text)
      onQuerySubmit?.(text)
      if (onSearchRequest) {
        onSearchRequest(request)
        return
      }
      if (!signal) return
      if (!request) {
        fetchClean(beginListUpdate(signal))
        return
      }

      const { predicate: queryTree, limit } = request
      if (!queryTree && limit === null) {
        fetchClean(beginListUpdate(signal))
        return
      }

      const searchCtx = currentSearchContext(signal)
      const sort = { field: sortValue, direction: sortDirection }
      const updateSeq = beginListUpdate(signal)
      executeSearch(
        runSearch(
          searchCtx,
          updateSeq,
          queryTree ?? undefined,
          limit ?? undefined,
          sort
        )
      )
    } catch (err) {
      searchError = err instanceof Error ? err.message : 'Parse error'
    }
  }

  function restoreQuery() {
    const text = editorView?.state.doc.toString() ?? ''
    if (!definitionsLoaded && text.trim()) {
      try {
        parseSearchRequest(text, [...staticFieldsList], signal)
      } catch {
        pendingRestoration = text
        return
      }
    }
    onSubmit()
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
    activeFilterID = activeFilterID === id ? null : id
  }

  /** Close filter popover when clicking outside. */
  $effect(() => {
    if (!activeFilterID) return
    const handleClick = (e: MouseEvent) => {
      const target = e.target
      if (!(target instanceof Node)) return
      if (filterPopoverEl?.contains(target)) return
      const btn =
        target instanceof Element && target.closest('[data-filter-id]')
      if (btn) return
      activeFilterID = null
    }
    document.addEventListener('click', handleClick, true)
    return () => document.removeEventListener('click', handleClick, true)
  })

  $effect(() => {
    onSearchError?.(searchError)
  })

  onMount(() => {
    const mountEl = editorContainer
    if (!mountEl) return

    // One cache for the whole editor session, shared by both value sources.
    const fieldValueCache = signal
      ? createFieldValueCache(telemetryAPI.getFieldValueCompletions, signal)
      : null

    const initialQuery = readRoute().query.q ?? ''
    routeQuery = initialQuery
    queryText = initialQuery
    const state = EditorState.create({
      doc: initialQuery,
      extensions: [
        queryLanguageSupport(),
        EditorView.contentAttributes.of({
          'aria-label': `Search ${definition?.label ?? signal}`,
        }),
        ...(signal ? [attributeTokens(signal)] : []),
        multilineQueryPaste,
        autocompletion({
          // Fetch values only after the user selects an exact field.
          override: [
            createQueryCompletionSource(() => availableFields, signal),
            ...(fieldValueCache
              ? [
                  // Column values and exact selected attribute paths.
                  createFieldValueSource(
                    fieldValueCache,
                    () => availableFields,
                    async (field, term) => {
                      const matches = await telemetryAPI.searchAttributeMatches(
                        term,
                        {
                          signal,
                          owner: field.owner,
                          path: field.path,
                          type: storedKindForField(field),
                        }
                      )
                      return matches.flatMap(match =>
                        match.sampleValues.flatMap(value =>
                          value.kind === 'map' || value.kind === 'array'
                            ? []
                            : [
                                value.kind === 'empty'
                                  ? 'null'
                                  : String(value.value),
                              ]
                        )
                      )
                    }
                  ),
                ]
              : []),
          ],
          activateOnTyping: true,
          icons: false,
        }),
        createQueryLinter(() => availableFields, signal),
        createQueryKeymap(onSubmit),
        ...queryTheme,
        tooltips({ position: 'fixed' }),
        closeBrackets(),
        history(),
        EditorView.updateListener.of(update => {
          if (update.docChanged) {
            pendingRestoration = undefined
            queryText = update.state.doc.toString()
          }
        }),
        keymap.of([...defaultKeymap, ...historyKeymap]),
        placeholderCompartment.of(cmPlaceholder(placeholderText)),
        // Single-line input: no soft-wrap, swallow Enter (we map it to submit
        // in createQueryKeymap; the keymap entry wins over Enter inserting a
        // newline because keymaps run before defaults). Keep horizontal
        // scroll for long queries instead of growing the editor vertically.
        EditorState.transactionFilter.of(tr =>
          tr.newDoc.lines > 1 ? [] : [tr]
        ),
      ],
    })

    editorView = new EditorView({
      state,
      parent: mountEl,
    })

    onReady?.({
      submit: onSubmit,
      clear: clearSearch,
      setQuery: text => {
        if (!editorView) return
        editorView.dispatch({
          changes: { from: 0, to: editorView.state.doc.length, insert: text },
        })
        onSubmit()
      },
    })
    if (initialQuery || definition) restoreQuery()
    return subscribeToRoute(() => {
      const route = readRoute()
      const path = definition?.path ?? `/${signal}`
      if (route.path !== path && !route.path.startsWith(`${path}/`)) return
      const next = route.query.q ?? ''
      if (next === routeQuery || !editorView) return
      routeQuery = next
      editorView.dispatch({
        changes: { from: 0, to: editorView.state.doc.length, insert: next },
      })
      restoreQuery()
    })
  })

  onDestroy(() => {
    alive = false
    if (signal) cancelPendingListUpdates(signal)
    editorView?.destroy()
  })
</script>

{#if variant === 'drawer'}
  <div class="search-editor-wrapper search-editor-wrapper--drawer">
    <div
      class="search-editor-container typed-field typed-field--pill input"
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
        <div class="header-icon-controls shrink-0">
          <button
            type="button"
            class="btn btn-circle btn-ghost btn-sm shrink-0 text-sm text-base-content tooltip tooltip-bottom"
            onclick={clearSearch}
            aria-label="Clear search"
            data-tip="Clear search"
          >
            <HugeiconsIcon
              icon={Cancel01Icon}
              size={17}
              strokeWidth={1.5}
              class="shrink-0"
              aria-hidden="true"
            />
          </button>
          <button
            type="button"
            class="btn btn-circle btn-ghost btn-sm shrink-0 text-sm text-base-content tooltip tooltip-bottom"
            onclick={openHelp}
            aria-label="Search query help"
            data-tip="Query help"
          >
            <QuestionMarkIcon
              class="h-[17px] w-[17px] shrink-0"
              aria-hidden="true"
            />
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
      class="search-editor-container typed-field input"
      class:search-editor-container--error={!!searchError}
    >
      <div class="editor-mount" bind:this={editorContainer}></div>

      <div class="search-actions join">
        <button
          type="button"
          class="btn btn-ghost btn-neutral btn-sm btn-square join-item tooltip tooltip-bottom"
          onclick={openHelp}
          aria-label="Search query help"
          data-tip="Query help"
        >
          <QuestionMarkIcon class="h-4 w-4 shrink-0" aria-hidden="true" />
        </button>
        <button
          type="button"
          class="btn btn-ghost btn-neutral btn-sm btn-square join-item tooltip tooltip-bottom"
          onclick={clearSearch}
          aria-label="Clear search"
          data-tip="Clear search"
        >
          <HugeiconsIcon
            icon={Cancel01Icon}
            size="1em"
            strokeWidth={1.5}
            class="h-4 w-4 shrink-0"
            aria-hidden="true"
          />
        </button>
        <button
          type="button"
          class="btn btn-ghost btn-neutral btn-sm btn-square join-item tooltip tooltip-bottom"
          onclick={onSubmit}
          aria-label="Search (Enter)"
          data-tip="Search (Enter)"
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
      <div
        bind:this={filterPopoverEl}
        class="popover-surface search-filter-popover"
      >
        {@render activeFilter.content()}
      </div>
    {/if}
  </div>
{/if}

<SearchHelpDialog bind:element={helpDialogElement} {signal} {definition} />

<style lang="postcss">
  @reference "../../../app.css";

  .search-editor-wrapper {
    @apply relative w-full;
    min-height: var(--table-row-h);
    line-height: var(--text-base--line-height);
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
    @apply items-center py-1 pl-4 pr-0;
    gap: var(--control-icon-gap);
    height: auto;
  }

  .search-editor__header-actions {
    @apply absolute top-1 left-2 right-2 z-10;
  }

  .search-editor-wrapper--drawer .editor-mount {
    @apply self-stretch;
  }

  .search-editor-wrapper--drawer .editor-mount :global(.cm-editor) {
    height: var(--search-editor-row-height, var(--table-row-h));
    min-height: var(--search-editor-row-height, var(--table-row-h));
    max-height: var(--search-editor-row-height, var(--table-row-h));
  }

  .search-editor-wrapper--drawer .editor-mount :global(.cm-scroller) {
    line-height: var(--text-base--line-height);
    overflow-x: auto;
    overflow-y: hidden;
  }

  .search-editor-wrapper--drawer .editor-mount :global(.cm-content) {
    padding-block: calc(
      (
          var(--search-editor-row-height, var(--table-row-h)) -
            var(--text-base) * var(--text-base--line-height)
        ) /
        2
    );
    padding-left: 0;
    padding-right: 0.5rem;
    min-height: var(--search-editor-row-height, var(--table-row-h));
    line-height: var(--text-base--line-height);
    white-space: nowrap;
  }

  .search-editor-wrapper--drawer .search-editor__footer-actions {
    @apply static ml-auto flex shrink-0 items-center gap-2;
    inset: auto;
    height: var(--search-editor-row-height, var(--table-row-h));
    align-self: center;
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

  .search-editor-container {
    @apply input relative flex w-full items-center;
    height: fit-content;
  }

  .search-editor-container:focus-within {
    outline: var(--focus-ring-width) solid var(--focus-ring-color);
    outline-offset: var(--focus-ring-offset);
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
    @apply px-3 py-2;
  }
</style>

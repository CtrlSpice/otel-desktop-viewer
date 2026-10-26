<script lang="ts" generics="TSortColumn extends string">
  import DateTimeFilter from '@/components/shared/Toolbar/DateTimeFilter.svelte'
  import SearchEditor from '@/components/shared/Search/SearchEditor.svelte'
  import type { SearchResultEvent } from '@/types/api-types'
  import type { SearchEditorAPI } from '@/components/shared/Search/search-editor-api'
  import type { FieldDefinition, ParsedSearchRequest } from '@/search/model'

  import type { SortDirection } from '@/contexts/signal-list-page.svelte'

  type DrawerSearchPanelSegment = 'full' | 'toolbar' | 'search'

  type Props = {
    /** `toolbar` = time range · `search` = editor · `full` = both */
    segment?: DrawerSearchPanelSegment
    signal: 'traces' | 'metrics' | 'logs'
    sortValue: TSortColumn
    sortDirection: SortDirection
    onSearchResults?: (event: SearchResultEvent) => void
    onSearchError?: (error: string | null) => void
    onSearchReady?: (api: SearchEditorAPI) => void
    onQueryChange?: (text: string, fields: FieldDefinition[]) => void
    onQuerySubmit?: (text: string) => void
    onSearchRequest?: (request: ParsedSearchRequest | null) => void
  }

  let {
    segment = 'full',
    signal,
    sortValue,
    sortDirection,
    onSearchResults,
    onSearchError,
    onSearchReady,
    onQueryChange,
    onQuerySubmit,
    onSearchRequest,
  }: Props = $props()
</script>

<div class="flex w-full min-w-0 flex-col gap-2">
  {#if segment === 'full' || segment === 'toolbar'}
    <div
      class="flex min-w-0 items-center justify-end gap-2"
      role="toolbar"
      aria-label="List controls"
    >
      <DateTimeFilter />
    </div>
  {/if}

  {#if segment === 'full' || segment === 'search'}
    <SearchEditor
      {signal}
      {sortValue}
      {sortDirection}
      variant="drawer"
      {onSearchResults}
      {onSearchError}
      {onQueryChange}
      {onQuerySubmit}
      {onSearchRequest}
      onReady={onSearchReady}
    />
  {/if}
</div>

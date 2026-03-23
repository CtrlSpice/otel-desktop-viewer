<script lang="ts">
  import DateTimeFilter from './datetime/DateTimeFilter.svelte';
  import SearchEditor from './search/SearchEditor.svelte';
  import type { SearchResultEvent } from '@/types/api-types';

  type PageHeaderProps =
    | {
        signal: 'logs';
        view: 'list';
        onRefresh?: (() => void) | null;
        onSearchResults?: ((event: SearchResultEvent) => void) | null;
      }
    | {
        signal: 'traces' | 'metrics';
        view: 'list' | 'detail';
        onRefresh?: (() => void) | null;
        onSearchResults?: ((event: SearchResultEvent) => void) | null;
      };

  let {
    signal,
    view,
    onRefresh = null,
    onSearchResults = null,
  }: PageHeaderProps = $props();
</script>

<!-- Page Header Component -->
<div class="mb-8 space-y-5">
  <!-- Title Row -->
  <div class="flex flex-wrap items-center gap-3">
    {#if onRefresh}
      <button
        type="button"
        class="btn btn-square btn-ghost btn-sm border border-base-300/60 text-base-content/80 hover:border-primary/35 hover:bg-primary/5 hover:text-primary"
        onclick={onRefresh}
        aria-label="Refresh"
      >
        <svg
          class="h-4 w-4"
          viewBox="0 0 24 24"
        >
          <path
            d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"
          ></path>
        </svg>
      </button>
    {/if}
    <h1 class="text-3xl font-semibold tracking-tight text-base-content">
      {signal.charAt(0).toUpperCase() + signal.slice(1)}
    </h1>
  </div>

  <!-- Search and Time Row: stack when narrow (half-screen), row when wide -->
  <div
    class="flex min-w-0 flex-col gap-3 min-[1000px]:flex-row min-[1000px]:items-center min-[1000px]:gap-4"
  >
    <DateTimeFilter />
    <div class="min-w-0 flex-1">
      {#if signal === 'logs'}
        <SearchEditor
          signal="logs"
          view="list"
          onSearchResults={onSearchResults || undefined}
        />
      {:else}
        <SearchEditor
          signal={signal}
          {view}
          onSearchResults={onSearchResults || undefined}
        />
      {/if}
    </div>
  </div>
</div>

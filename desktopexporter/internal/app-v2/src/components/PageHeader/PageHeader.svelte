<script lang="ts">
  import DateTimeFilter from './filters/datetime/DateTimeFilter.svelte';
  import SearchInput from './search/SearchInput.svelte';
  import type { SearchResultEvent } from '@/types/api-types';

  let {
    signal,
    view,
    onRefresh = null,
    onSearchResults = null,
  }: {
    signal: 'traces' | 'logs' | 'metrics';
    view: 'list' | 'detail';
    onRefresh?: (() => void) | null;
    onSearchResults?: ((event: SearchResultEvent) => void) | null;
  } = $props();
</script>

<!-- Page Header Component -->
<div class="mb-6 space-y-4">
  <!-- Title Row -->
  <div class="flex items-center gap-3">
    {#if onRefresh}
      <button
        class="btn btn-circle btn-primary btn-sm"
        onclick={onRefresh}
        aria-label="Refresh"
      >
        <svg
          class="w-4 h-4"
          viewBox="0 0 24 24"
        >
          <path
            d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"
          ></path>
        </svg>
      </button>
    {/if}
    <h1 class="text-2xl font-bold">
      {signal.charAt(0).toUpperCase() + signal.slice(1)}
    </h1>
  </div>

  <!-- Search and Time Row -->
  <div class="flex items-center gap-4">
    <DateTimeFilter />
    <div class="flex-1">
      <SearchInput {signal} {view} onSearchResults={onSearchResults || undefined} />
    </div>
  </div>
</div>

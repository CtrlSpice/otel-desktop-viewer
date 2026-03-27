<script lang="ts">
  import DateTimeFilter from './datetime/DateTimeFilter.svelte';
  import SearchEditor from './search/SearchEditor.svelte';
  import type { SearchResultEvent } from '@/types/api-types';

  type SignalHeaderProps =
    | {
        signal: 'logs';
        view: 'list';
        /** When set, replaces the default capitalized signal name in the title. */
        title?: string | undefined;
        onRefresh?: (() => void) | null;
        onSearchResults?: ((event: SearchResultEvent) => void) | null;
      }
    | {
        signal: 'traces' | 'metrics';
        view: 'list' | 'detail';
        title?: string | undefined;
        onRefresh?: (() => void) | null;
        onSearchResults?: ((event: SearchResultEvent) => void) | null;
      };

  let {
    signal,
    view,
    title: pageTitle = undefined,
    onRefresh = null,
    onSearchResults = null,
  }: SignalHeaderProps = $props();
</script>

<!-- Page Header Component — px-1 so focus rings on refresh / time control aren’t clipped at the content edge -->
<div class="mb-8 space-y-4 px-1">
  <!-- Refresh, title, and time on one row (time full-width row below title on xs; right-aligned from sm) -->
  <div class="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-2">
    {#if onRefresh}
      <button
        type="button"
        class="btn btn-circle btn-primary btn-sm shrink-0"
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
    <h1 class="min-w-0 shrink text-3xl font-semibold tracking-tight text-base-content">
      {pageTitle ?? signal.charAt(0).toUpperCase() + signal.slice(1)}
    </h1>
    <div
      class="min-w-0 w-full basis-full sm:w-auto sm:basis-auto sm:ml-auto sm:max-w-lg md:max-w-xl"
    >
      <DateTimeFilter />
    </div>
  </div>

  <!-- Search fills its own row -->
  <div class="min-w-0 w-full">
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

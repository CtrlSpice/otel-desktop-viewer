<script lang="ts">
  import DateTimeFilter from './datetime/DateTimeFilter.svelte';
  import SearchEditor from './search/SearchEditor.svelte';
  import type { SearchResultEvent } from '@/types/api-types';

  type SignalHeaderProps =
    | {
        signal: 'traces' | 'metrics' | 'logs';
        view: 'list';
        onRefresh?: (() => void) | null;
        onSearchResults?: ((event: SearchResultEvent) => void) | null;
      }
    | {
        signal: 'traces';
        view: 'detail';
        traceID: string;
        onBack?: (() => void) | null;
        onRefresh?: (() => void) | null;
        onSearchResults?: ((event: SearchResultEvent) => void) | null;
      }
    | {
        signal: 'metrics';
        view: 'detail';
        metricName: string;
        onBack?: (() => void) | null;
        onRefresh?: (() => void) | null;
        onSearchResults?: ((event: SearchResultEvent) => void) | null;
      };

  let props: SignalHeaderProps = $props();

  let signal = $derived(props.signal);
  let view = $derived(props.view);
  let onBack = $derived('onBack' in props ? (props.onBack ?? null) : null);
  let onRefresh = $derived(props.onRefresh ?? null);
  let onSearchResults = $derived(props.onSearchResults ?? null);

  let pageTitle = $derived.by(() => {
    if ('traceID' in props) return props.traceID
    if ('metricName' in props) return props.metricName
    return signal.charAt(0).toUpperCase() + signal.slice(1)
  });
</script>

<!-- Page Header Component — px-1 so focus rings on refresh / time control aren’t clipped at the content edge -->
<div class="mb-8 space-y-4 px-1">
  <!-- Refresh, title, and time on one row (time full-width row below title on xs; right-aligned from sm) -->
  <div class="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-2">
    {#if onBack}
      <button
        type="button"
        class="btn btn-circle btn-outline btn-sm shrink-0 border-base-300/80 text-base-content/70 hover:border-base-content/25 hover:bg-base-200/60 hover:text-base-content"
        onclick={onBack}
        aria-label="Go back"
      >
        <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5">
          <path d="M11 6h4.5a4.5 4.5 0 1 1 0 9H4" />
          <path d="M7 12s-3 2.21-3 3s3 3 3 3" />
        </svg>
      </button>
    {/if}
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
    <h1 class="min-w-0 shrink truncate font-mono text-lg font-semibold tracking-normal text-base-content/55" title={pageTitle}>
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
    {#if signal === 'traces' && view === 'detail' && 'traceID' in props}
      <SearchEditor
        signal="traces"
        view="detail"
        traceID={props.traceID}
        onSearchResults={onSearchResults || undefined}
      />
    {:else if view === 'detail'}
      <SearchEditor
        signal={signal as 'metrics'}
        view="detail"
        onSearchResults={onSearchResults || undefined}
      />
    {:else}
      <SearchEditor
        {signal}
        view="list"
        onSearchResults={onSearchResults || undefined}
      />
    {/if}
  </div>
</div>

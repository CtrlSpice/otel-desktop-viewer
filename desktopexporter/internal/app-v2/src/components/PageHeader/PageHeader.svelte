<script lang="ts">
  import DateTimeFilter from './filters/datetime/DateTimeFilter.svelte';
  import FieldsFilter from './filters/fields/FieldsFilter.svelte';
  import SearchInput from './search/SearchInput.svelte';

  let {
    signal,
    view,
    onRefresh = null,
  }: {
    signal: 'traces' | 'logs' | 'metrics';
    view: 'list' | 'detail';
    onRefresh?: (() => void) | null;
  } = $props();

  let showFieldsDrawer = $state(false);
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
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
        >
          <path
            stroke-linecap="round"
            stroke-linejoin="round"
            stroke-width="2"
            d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"
          ></path>
        </svg>
      </button>
    {/if}
    <h1 class="text-2xl font-bold">
      {signal.charAt(0).toUpperCase() + signal.slice(1)}
    </h1>
  </div>

  <!-- Search Row -->
  <div>
    <SearchInput {signal} />
  </div>

  <!-- Control Row: Hints, Time, Fields -->
  <div class="flex items-center gap-4">
    <!-- DateTime Filter -->
    <DateTimeFilter />

    

    <!-- Fields View Button -->
    <div class="tooltip tooltip-bottom md:hidden" data-tip="Fields">
      <button
        class="btn btn-circle btn-sm md:hidden"
        onclick={() => (showFieldsDrawer = !showFieldsDrawer)}
        aria-label="Fields"
      >
        <svg
          class="w-4 h-4"
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
        >
          <path
            stroke-linecap="round"
            stroke-linejoin="round"
            stroke-width="2"
            d="M9 17V7m0 10a2 2 0 01-2 2H5a2 2 0 01-2-2V7a2 2 0 012-2h2a2 2 0 012 2m0 10a2 2 0 002 2h2a2 2 0 002-2M9 7a2 2 0 012-2h2a2 2 0 012 2m0 10V7m0 10a2 2 0 002 2h2a2 2 0 002-2V7a2 2 0 00-2-2h-2a2 2 0 00-2 2"
          ></path>
        </svg>
      </button>

      <!-- Hints (Empty for now) -->
    <div class="flex-1">
      <!-- TODO: Add hints here -->
    </div>
    </div>

    <button
      class="input input-bordered input-sm items-center gap-2 hidden md:inline-flex"
      onclick={() => (showFieldsDrawer = !showFieldsDrawer)}
    >
      <svg
        class="w-4 h-4"
        fill="none"
        stroke="currentColor"
        viewBox="0 0 24 24"
      >
        <path
          stroke-linecap="round"
          stroke-linejoin="round"
          stroke-width="2"
          d="M9 17V7m0 10a2 2 0 01-2 2H5a2 2 0 01-2-2V7a2 2 0 012-2h2a2 2 0 012 2m0 10a2 2 0 002 2h2a2 2 0 002-2M9 7a2 2 0 012-2h2a2 2 0 012 2m0 10V7m0 10a2 2 0 002 2h2a2 2 0 002-2V7a2 2 0 00-2-2h-2a2 2 0 00-2 2"
        ></path>
      </svg>
      <span>Fields</span>
      <svg
        class="w-3 h-3 transition-transform duration-200 {showFieldsDrawer
          ? 'rotate-180'
          : ''}"
        fill="none"
        stroke="currentColor"
        viewBox="0 0 24 24"
      >
        <path
          stroke-linecap="round"
          stroke-linejoin="round"
          stroke-width="2"
          d="M19 9l-7 7-7-7"
        ></path>
      </svg>
    </button>
  </div>

  <!-- Fields Drawer -->
  {#if showFieldsDrawer}
    <div class="bg-base-100 border border-base-300 py-2 rounded">
      <FieldsFilter />
    </div>
  {/if}
</div>

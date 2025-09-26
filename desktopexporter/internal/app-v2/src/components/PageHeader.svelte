<script lang="ts">
  import DateTimeFilter from './filters/datetime/DateTimeFilter.svelte';
  import AttributeFilter from './AttributeFilter.svelte';
  import FieldsFilter from './FieldsFilter.svelte';
  import { getTimeContext } from '@/contexts/time-context.svelte';
  import type { TelemetryFilters } from '@/types/filter-types';
  import { formatDateTimeRange } from '@/utils/time';

  let { 
    title, 
    filters, 
    onRefresh = null, 
    onFiltersChange = null, 
  }: {
    title: string;
    filters: TelemetryFilters;
    onRefresh?: (() => void) | null;
    onFiltersChange?: ((filters: TelemetryFilters) => void) | null;
  } = $props();

  let showDateTimeDrawer = $state(false);
  let showFiltersDrawer = $state(false);
  let showFieldsDrawer = $state(false);

  // Get time context to listen for changes
  let timeContext = getTimeContext();
  
  // Track selection changes with derived rune
  let currentSelection = $derived(timeContext.selection);
  let previousSelection = $state<string | null>(null);
  
  $effect(() => {
    let currentSelectionString = JSON.stringify($state.snapshot(currentSelection));
    console.log('Selection changed:', $state.snapshot(currentSelection));
    
    // Close drawer if selection actually changed and drawer is open
    if (previousSelection && previousSelection !== currentSelectionString && showDateTimeDrawer) {
      showDateTimeDrawer = false;
    }
    
    previousSelection = currentSelectionString;
  });

  // Get display text for current time selection - always show absolute time range
  function getDisplayText(ctx: any): string {
    if (!ctx?.selection) {
      return 'Select time range';
    }

    return formatDateTimeRange(ctx.selection.start, ctx.selection.end, ctx.timezone);
  }

  // Handle attribute filter changes
  function handleAttributesChange(attributes: any[]) {
    let newFilters = {
      ...filters,
      attributes,
    };
    onFiltersChange?.(newFilters);
  }


  // Handle search input changes
  function handleSearchChange(event: Event) {
    let target = event.target as HTMLInputElement;
    let newFilters = {
      ...filters,
      search: target.value,
    };
    onFiltersChange?.(newFilters);
  }


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
    <h1 class="text-2xl font-bold">{title}</h1>
  </div>

  <!-- Control Row -->
  <div class="flex items-center gap-4">
    <!-- Time Filter Button -->
    <div class="tooltip tooltip-bottom tooltip-bottom-right md:hidden" data-tip="{getDisplayText(timeContext)}">
      <button
        class="btn btn-circle btn-sm md:hidden"
        onclick={() => (showDateTimeDrawer = !showDateTimeDrawer)}
        aria-label="Time Filter"
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
            d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"
          ></path>
        </svg>
      </button>
    </div>
    
    <button
      class="input input-bordered input-sm items-center gap-2 hidden md:inline-flex"
      onclick={() => (showDateTimeDrawer = !showDateTimeDrawer)}
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
          d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"
        ></path>
      </svg>
      <span>{getDisplayText(timeContext)}</span>
      <svg
        class="w-3 h-3 transition-transform duration-200 {showDateTimeDrawer
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

    <!-- Search Bar -->
    <div class="form-control flex-1">
      <div class="relative">
        <svg
          class="absolute left-3 top-1/2 transform -translate-y-1/2 w-4 h-4 text-base-content/60"
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
        >
          <path
            stroke-linecap="round"
            stroke-linejoin="round"
            stroke-width="2"
            d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"
          ></path>
        </svg>
        <input
          id="search-input"
          type="text"
          placeholder="Search traces..."
          class="input input-bordered input-sm pl-10 w-full"
          value={filters.search}
          oninput={handleSearchChange}
        />
      </div>
    </div>

    <!-- Filters Button -->
    <div class="tooltip tooltip-bottom md:hidden" data-tip="Filters">
      <button
        class="btn btn-circle btn-sm md:hidden"
        onclick={() => (showFiltersDrawer = !showFiltersDrawer)}
        aria-label="Filters"
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
            d="M3 4a1 1 0 011-1h16a1 1 0 011 1v2.586a1 1 0 01-.293.707l-6.414 6.414a1 1 0 00-.293.707V17l-4 4v-6.586a1 1 0 00-.293-.707L3.293 7.293A1 1 0 013 6.586V4z"
          ></path>
        </svg>
      </button>
    </div>
    
    <button
      class="input input-bordered input-sm items-center gap-2 hidden md:inline-flex"
      onclick={() => (showFiltersDrawer = !showFiltersDrawer)}
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
          d="M3 4a1 1 0 011-1h16a1 1 0 011 1v2.586a1 1 0 01-.293.707l-6.414 6.414a1 1 0 00-.293.707V17l-4 4v-6.586a1 1 0 00-.293-.707L3.293 7.293A1 1 0 013 6.586V4z"
        ></path>
      </svg>
      <span>Filters</span>
      <svg
        class="w-3 h-3 transition-transform duration-200 {showFiltersDrawer
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

  <!-- DateTime Drawer -->
  {#if showDateTimeDrawer}
    <div class="bg-base-100 border border-base-300 py-2 rounded">
      <DateTimeFilter />
    </div>
  {/if}

  <!-- Filters Drawer -->
  {#if showFiltersDrawer}
    <div class="bg-base-100 border border-base-300 py-2 rounded">
      <AttributeFilter
        attributes={filters.attributes || []}
        onAttributesChange={handleAttributesChange}
      />
    </div>
  {/if}

    <!-- Fields Drawer -->
    {#if showFieldsDrawer}
      <div class="bg-base-100 border border-base-300 py-2 rounded">
        <FieldsFilter />
      </div>
    {/if}
</div>

<style>
  /* Custom tooltip positioning for bottom-right */
.tooltip-bottom-right {
  position: relative;
}

.tooltip-bottom-right::before {
  transform: translateX(-10px) !important;
}
</style>
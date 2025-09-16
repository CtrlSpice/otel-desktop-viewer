<script lang="ts">
  import DateTimeFilter from './DateTimeFilter.svelte';
  import type { TraceFilters, AttributeFilter } from '../types/filter-types';
  import {
    ALL_ATTRIBUTE_SUGGESTIONS,
    searchSuggestions,
  } from '../constants/semantic-conventions';

  export let title: string;
  export let filters: TraceFilters;
  export let onRefresh: (() => void) | null = null;
  export let onFiltersChange: ((filters: TraceFilters) => void) | null = null;
  export let onTimezoneChange: ((timezone: string) => void) | null = null;

  // State for drawers
  let showTimeDrawer = false;
  let showFiltersDrawer = false;
  let showFieldsDrawer = false;

  // Attribute filter helpers
  function addAttributeFilter() {
    const newFilters = {
      ...filters,
      attributes: [
        ...(filters.attributes || []),
        { name: '', value: '', operator: 'equals' as const },
      ],
    };
    if (onFiltersChange) onFiltersChange(newFilters);
  }

  function removeAttributeFilter(index: number) {
    const newFilters = {
      ...filters,
      attributes: filters.attributes?.filter((_, i) => i !== index) || [],
    };
    if (onFiltersChange) onFiltersChange(newFilters);
  }

  function clearAllFilters() {
    const clearedFilters: TraceFilters = {
      search: '',
      serviceName: [],
      timeRange: {
        start: '',
        end: '',
      },
      attributes: [],
    };
    if (onFiltersChange) onFiltersChange(clearedFilters);
  }

  // Search for attribute suggestions
  function getAttributeSuggestions(query: string) {
    if (!query) return ALL_ATTRIBUTE_SUGGESTIONS.slice(0, 10); // Show first 10 when empty
    return searchSuggestions(query).slice(0, 10); // Limit to 10 results
  }

  // Handle datetime filter changes
  function handleDateTimeChange(
    event: CustomEvent<{ start?: string; end?: string }>
  ) {
    const newFilters = {
      ...filters,
      timeRange: {
        start: event.detail.start || '',
        end: event.detail.end || '',
      },
    };
    if (onFiltersChange) onFiltersChange(newFilters);
    showTimeDrawer = false; // Close drawer after selection
  }

  function handleTimezoneChange(event: CustomEvent<string>) {
    const newTimezone = event.detail;
    if (onTimezoneChange) onTimezoneChange(newTimezone);
  }

  // Handle search input changes
  function handleSearchChange(event: Event) {
    const target = event.target as HTMLInputElement;
    const newFilters = {
      ...filters,
      search: target.value,
    };
    if (onFiltersChange) onFiltersChange(newFilters);
  }

  // Get display text for time range
  function getTimeDisplayText(): string {
    if (!filters.timeRange.start && !filters.timeRange.end)
      return 'Last 15 minutes';

    const start = filters.timeRange.start
      ? new Date(filters.timeRange.start)
      : null;
    const end = filters.timeRange.end ? new Date(filters.timeRange.end) : null;

    if (start && end) {
      const diffMs = end.getTime() - start.getTime();
      const diffMinutes = Math.round(diffMs / (1000 * 60));

      if (diffMinutes <= 5) return 'Last 5 minutes';
      if (diffMinutes <= 15) return 'Last 15 minutes';
      if (diffMinutes <= 30) return 'Last 30 minutes';
      if (diffMinutes <= 60) return 'Last hour';
      if (diffMinutes <= 360) return 'Last 6 hours';
      if (diffMinutes <= 1440) return 'Last day';
      if (diffMinutes <= 4320) return 'Last 3 days';
      if (diffMinutes <= 10080) return 'Last week';
    }

    // Show actual time range for custom ranges
    if (start && end) {
      const formatTime = (date: Date) => {
        return date.toLocaleString('en-US', {
          month: 'short',
          day: 'numeric',
          hour: '2-digit',
          minute: '2-digit',
          hour12: false,
        });
      };
      return `${formatTime(start)} - ${formatTime(end)}`;
    }

    return 'Custom range';
  }
</script>

<!-- Page Header Component -->
<div class="mb-6 space-y-4">
  <!-- Title Row -->
  <div class="flex items-center gap-3">
    {#if onRefresh}
      <button
        class="btn btn-circle btn-primary btn-sm"
        on:click={onRefresh}
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
    <button
      class="input input-bordered input-sm flex items-center gap-2"
      on:click={() => (showTimeDrawer = !showTimeDrawer)}
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
      <span>{getTimeDisplayText()}</span>
      <svg
        class="w-3 h-3 transition-transform duration-200 {showTimeDrawer
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

    <!-- Filters Button -->
    <button
      class="input input-bordered input-sm flex items-center gap-2"
      on:click={() => (showFiltersDrawer = !showFiltersDrawer)}
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
          on:input={handleSearchChange}
        />
      </div>
    </div>

    <!-- Fields View Button -->
    <button
      class="input input-bordered input-sm flex items-center gap-2"
      on:click={() => (showFieldsDrawer = !showFieldsDrawer)}
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

  <!-- Time Filter Drawer -->
  {#if showTimeDrawer}
    <div class="bg-base-100 border border-base-300 rounded p-4">
      <div class="flex gap-6">
        <!-- Left Side - Absolute Time Range -->
        <div class="w-80 space-y-4">
          <div class="text-sm font-medium text-base-content">
            Absolute time range
          </div>

          <!-- From Date/Time -->
          <div class="form-control">
            <label class="label" for="custom-start">
              <span class="label-text text-sm">From</span>
            </label>
            <input
              id="custom-start"
              type="text"
              placeholder="e.g., 2 hours ago, yesterday, 2024-01-01"
              class="input input-bordered input-sm w-full"
            />
          </div>

          <!-- To Date/Time -->
          <div class="form-control">
            <label class="label" for="custom-end">
              <span class="label-text text-sm">To</span>
            </label>
            <input
              id="custom-end"
              type="text"
              placeholder="e.g., now, 1 hour ago, 2024-01-02"
              class="input input-bordered input-sm w-full"
            />
          </div>

          <!-- Apply Button -->
          <button class="btn btn-primary btn-sm w-full">
            Apply time range
          </button>
        </div>

        <!-- Right Side - Preset Time Ranges -->
        <div class="w-80 space-y-4">
          <div class="text-sm font-medium text-base-content">
            Preset time ranges
          </div>

          <!-- Preset Options -->
          <div class="space-y-1">
            <button
              class="w-full text-left px-3 py-2 text-sm hover:bg-base-200 transition-colors"
            >
              Show all
            </button>
            <button
              class="w-full text-left px-3 py-2 text-sm hover:bg-base-200 transition-colors"
            >
              Last 5 minutes
            </button>
            <button
              class="w-full text-left px-3 py-2 text-sm hover:bg-base-200 transition-colors"
            >
              Last 15 minutes
            </button>
            <button
              class="w-full text-left px-3 py-2 text-sm hover:bg-base-200 transition-colors"
            >
              Last 30 minutes
            </button>
            <button
              class="w-full text-left px-3 py-2 text-sm hover:bg-base-200 transition-colors"
            >
              Last hour
            </button>
            <button
              class="w-full text-left px-3 py-2 text-sm hover:bg-base-200 transition-colors"
            >
              Last 6 hours
            </button>
            <button
              class="w-full text-left px-3 py-2 text-sm hover:bg-base-200 transition-colors"
            >
              Last day
            </button>
            <button
              class="w-full text-left px-3 py-2 text-sm hover:bg-base-200 transition-colors"
            >
              Last 3 days
            </button>
            <button
              class="w-full text-left px-3 py-2 text-sm hover:bg-base-200 transition-colors"
            >
              Last week
            </button>
          </div>
        </div>
      </div>
    </div>
  {/if}

  <!-- Filters Drawer -->
  {#if showFiltersDrawer}
    <div class="bg-base-100 border border-base-300 rounded p-4">
      <!-- Attributes and Fields Filter -->
      <div class="form-control">
        <div class="flex justify-between items-center mb-2">
          <span class="text-sm font-medium text-base-content/80"
            >Attributes and Fields</span
          >
          <div class="flex gap-2">
            <button
              class="btn btn-outline btn-xs"
              on:click={addAttributeFilter}
            >
              + Add filter
            </button>
            <button
              class="text-xs text-base-content/60 hover:text-base-content underline"
              on:click={clearAllFilters}
            >
              Clear all
            </button>
          </div>
        </div>

        {#each filters.attributes || [] as attrFilter, index}
          <div class="flex gap-2 items-end mb-2">
            <div class="form-control flex-1">
              <label class="label" for="attr-name-{index}">
                <span class="label-text text-xs">Attribute Name</span>
              </label>
              <input
                id="attr-name-{index}"
                type="text"
                placeholder="e.g., http.status_code"
                class="input input-bordered input-sm"
                value={attrFilter.name}
                on:input={e => {
                  const target = e.target as HTMLInputElement;
                  const newFilters = {
                    ...filters,
                    attributes:
                      filters.attributes?.map((attr, i) =>
                        i === index ? { ...attr, name: target.value } : attr
                      ) || [],
                  };
                  if (onFiltersChange) onFiltersChange(newFilters);
                }}
                list="attribute-suggestions-{index}"
              />
              <datalist id="attribute-suggestions-{index}">
                {#each getAttributeSuggestions(attrFilter.name) as suggestion}
                  <option
                    value={suggestion.name}
                    title={suggestion.description}
                  >
                    {suggestion.name} ({suggestion.category})
                  </option>
                {/each}
              </datalist>
            </div>

            <div class="form-control w-32">
              <label class="label" for="attr-operator-{index}">
                <span class="label-text text-xs">Operator</span>
              </label>
              <select
                id="attr-operator-{index}"
                class="select select-bordered select-sm"
                value={attrFilter.operator}
                on:change={e => {
                  const target = e.target as HTMLSelectElement;
                  const newFilters = {
                    ...filters,
                    attributes:
                      filters.attributes?.map((attr, i) =>
                        i === index
                          ? {
                              ...attr,
                              operator: target.value as
                                | 'equals'
                                | 'contains'
                                | 'startsWith',
                            }
                          : attr
                      ) || [],
                  };
                  if (onFiltersChange) onFiltersChange(newFilters);
                }}
              >
                <option value="equals">equals</option>
                <option value="contains">contains</option>
                <option value="startsWith">starts with</option>
              </select>
            </div>

            <div class="form-control flex-1">
              <label class="label" for="attr-value-{index}">
                <span class="label-text text-xs">Value</span>
              </label>
              <input
                id="attr-value-{index}"
                type="text"
                placeholder="Filter value"
                class="input input-bordered input-sm"
                value={attrFilter.value}
                on:input={e => {
                  const target = e.target as HTMLInputElement;
                  const newFilters = {
                    ...filters,
                    attributes:
                      filters.attributes?.map((attr, i) =>
                        i === index ? { ...attr, value: target.value } : attr
                      ) || [],
                  };
                  if (onFiltersChange) onFiltersChange(newFilters);
                }}
              />
            </div>

            <button
              class="btn btn-error btn-sm"
              on:click={() => removeAttributeFilter(index)}
            >
              ×
            </button>
          </div>
        {/each}

        {#if !filters.attributes || filters.attributes.length === 0}
          <div class="text-center py-4 text-base-content/60">
            <p>No attribute filters added</p>
            <p class="text-sm">
              Click "Add Filter" to search by span attributes
            </p>
          </div>
        {/if}
      </div>
    </div>
  {/if}
</div>

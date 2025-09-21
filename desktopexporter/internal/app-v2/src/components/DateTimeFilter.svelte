<script lang="ts">
  import * as chrono from 'chrono-node';
  import { getTimeContext } from '../contexts/time-context.svelte';
  import { 
    formatDateTime, 
    formatDateTimeRange, 
    getLocalTimezoneName,
    type Timezone
  } from '../utils/time';

  // Get time context
  let timeContext = getTimeContext();
  if (!timeContext) {
    throw new Error(
      'Time context not found. Make sure createTimeContext() is called at the root level.'
    );
  }

  export function onOpen() {
    // Get fresh time context
    let timeContext = getTimeContext();
    if (!timeContext) {
      throw new Error('Time context not found');
    }
    
    let saved = localStorage.getItem('datetime-filter-recent');
    if (saved) {
      recentTimeRanges = JSON.parse(saved);
    } else {
      recentTimeRanges = [];
    }
    
    switch (timeContext.selection.type) {
      case 'preset':
        presetIndex = timeContext.selection.presetIndex;
        break;
      case 'custom':
        customStartText = formatDateTime(
          timeContext.selection.start,
          timeContext.timezone,
          'seconds'
        );
        customEndText = formatDateTime(
          timeContext.selection.end,
          timeContext.timezone,
          'seconds'
        );
        customError = null;
        break;
      case 'recent':
        // No additional setup needed here
        break;
    }
    timezone = timeContext.timezone;
  }

  // ===== PRESET TIME RANGES =====
  const PRESETS = [
    { label: 'Last 5 minutes', duration: 300000 }, // 5 * 60 * 1000
    { label: 'Last 15 minutes', duration: 900000 }, // 15 * 60 * 1000
    { label: 'Last 30 minutes', duration: 1800000 }, // 30 * 60 * 1000
    { label: 'Last hour', duration: 3600000 }, // 60 * 60 * 1000
    { label: 'Last 6 hours', duration: 21600000 }, // 6 * 60 * 60 * 1000
    { label: 'Last day', duration: 86400000 }, // 24 * 60 * 60 * 1000
    { label: 'Last 3 days', duration: 259200000 }, // 3 * 24 * 60 * 60 * 1000
    { label: 'Last week', duration: 604800000 }, // 7 * 24 * 60 * 60 * 1000
    { label: 'Show all', duration: undefined },
  ] as const;

  // Preset time range index
  let presetIndex = $state<number | null>(null);

  function applyPreset(index: number) {
    let start = 0;
    let now = Date.now();
    let preset = PRESETS[index];

    // Handle "Show all" case
    if (preset.duration !== undefined) {
      start = now - preset.duration;
    }

    timeContext.setSelection(start, now, 'preset', index);
  }

  // ===== CUSTOM TIME RANGE =====
  let customStartText = $state('');
  let customEndText = $state('');
  let customError = $state<string | null>(null);

  type ValidationResult =
    | { isValid: true; start: number; end: number }
    | { isValid: false; error: string };

  type ParseResult =
    | { success: true; timestamp: number }
    | { success: false; error: string };

  function validateCustomRange(): ValidationResult {
    // Make sure neither start nor end is empty
    if (!customStartText.trim() || !customEndText.trim()) {
      return {
        isValid: false,
        error: 'Please enter both start and end times',
      };
    }

    // Parse start and end times
    let startResult = parseNaturalLanguage(customStartText);
    if (!startResult.success) {
      return {
        isValid: false,
        error: startResult.error,
      };
    }

    let endResult = parseNaturalLanguage(customEndText);
    if (!endResult.success) {
      return {
        isValid: false,
        error: endResult.error,
      };
    }

    let startTime = startResult.timestamp;
    let endTime = endResult.timestamp;

    // Validate start is before end
    if (startTime >= endTime) {
      return {
        isValid: false,
        error: 'Start time must be before end time',
      };
    }

    // Validate not in the future
    if (endTime > Date.now()) {
      return {
        isValid: false,
        error: 'End time cannot be in the future',
      };
    }

    return { isValid: true, start: startTime, end: endTime };
  }

  function applyCustom() {
    let now = Date.now();
    customError = null;
    let validation = validateCustomRange();

    if (!validation.isValid) {
      customError = validation.error;
      return; // Don't change the application state
    }

    // Add to recent entries
    updateRecentRanges(validation.start, validation.end, now);

    // Set selection in time context
    timeContext.setSelection(validation.start, validation.end, 'custom');
  }

  // ===== RECENT TIME RANGES =====
  interface RecentTimeRange {
    start: number;
    end: number;
    usedAt: number;
  }

  // Recently used time ranges
  let recentTimeRanges = $state<RecentTimeRange[]>([]);

  function updateRecentRanges(start: number, end: number, usedAt: number) {
    // Check if this time range already exists
    let existingIndex = recentTimeRanges.findIndex(
      entry => entry.start === start && entry.end === end
    );

    if (existingIndex !== -1) {
      // Update existing entry
      let updated = [...recentTimeRanges];
      updated[existingIndex] = { ...updated[existingIndex], usedAt };
      let sorted = updated.sort((a, b) => b.usedAt - a.usedAt);
      recentTimeRanges = sorted;
    } else {
      // Add new entry
      let updated = [{ start, end, usedAt }, ...recentTimeRanges]
        .sort((a, b) => b.usedAt - a.usedAt)
        .slice(0, 10);
      recentTimeRanges = updated;
    }

    // Save to localStorage
    localStorage.setItem(
      'datetime-filter-recent',
      JSON.stringify(recentTimeRanges)
    );
  }

  function applyRecentTimeRange(index: number) {
    let entry = recentTimeRanges[index];
    if (!entry) return;

    let now = Date.now();

    // First: Update the recent ranges (move to top)
    updateRecentRanges(entry.start, entry.end, now);

    // Then: Update the time context
    timeContext.setSelection(entry.start, entry.end, 'recent');
  }

  // ===== TIMEZONE =====
  let timezone = $state<Timezone>('local');

  // ===== FORMATTING AND UTILITY FUNCTIONS =====
  function parseNaturalLanguage(text: string): ParseResult {
    if (!text.trim()) {
      return { success: false, error: 'Please enter a time' };
    }

    try {
      let parsed = chrono.parseDate(text);
      if (parsed) {
        return { success: true, timestamp: parsed.getTime() };
      } else {
        return {
          success: false,
          error: 'Could not understand this time format',
        };
      }
    } catch (error) {
      return { success: false, error: 'Invalid time format' };
    }
  }


  // Export function to get display text for current selection
  export function getDisplayText(): string {
    let selection = timeContext.selection;

    // If it's a preset, show the preset name
    if (selection.type === 'preset') {
      return PRESETS[selection.presetIndex].label;
    }

    // For custom ranges, show the actual time range
    return formatDateTimeRange(selection.start, selection.end, timezone);
  }
</script>

<div class="relative">
  <!-- Drawer Content -->
  <div class="w-full">
    <div class="flex gap-3">
      <!-- Left Side - Preset Time Ranges -->
      <div class="space-y-4">
        <!-- Preset Options -->
        <div class="space-y-0">
          {#each PRESETS as preset, index}
            <button
              class="w-full text-left px-2 py-1 text-sm hover:bg-base-200 transition-colors flex items-center gap-2"
              onclick={() => applyPreset(index)}
            >
              {#if presetIndex === index}
                <svg
                  class="w-4 h-4 text-primary"
                  fill="currentColor"
                  viewBox="0 0 20 20"
                >
                  <path
                    fill-rule="evenodd"
                    d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z"
                    clip-rule="evenodd"
                  ></path>
                </svg>
              {:else}
                <div class="w-4 h-4"></div>
              {/if}
              <span>{preset.label}</span>
              <div class="w-4 h-4"></div>
            </button>
          {/each}
        </div>
      </div>

      <!-- Vertical separator -->
      <div class="w-px bg-base-300"></div>

      <!-- Middle Section - Custom Time Range -->
      <div class="flex-1 space-y-4">
        <div class="text-sm font-medium text-base-content">
          Custom time range
        </div>

        <!-- From Date/Time -->
        <div class="form-control">
          <label class="label" for="custom-start">
            <span class="label-text text-sm">Start time</span>
          </label>
          <input
            id="custom-start"
            type="text"
            placeholder="e.g., 2 hours ago, yesterday, 2024-01-01"
            class="input input-bordered input-sm w-full"
            bind:value={customStartText}
          />
        </div>

        <!-- To Date/Time -->
        <div class="form-control">
          <label class="label" for="custom-end">
            <span class="label-text text-sm">End time</span>
          </label>
          <input
            id="custom-end"
            type="text"
            placeholder="e.g., now, 1 hour ago, 2024-01-02"
            class="input input-bordered input-sm w-full"
            bind:value={customEndText}
          />
        </div>

        <!-- Error Display -->
        {#if customError}
          <div class="alert alert-error alert-sm">
            <svg class="w-4 h-4" fill="currentColor" viewBox="0 0 20 20">
              <path
                fill-rule="evenodd"
                d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7 4a1 1 0 11-2 0 1 1 0 012 0zm-1-9a1 1 0 00-1 1v4a1 1 0 102 0V6a1 1 0 00-1-1z"
                clip-rule="evenodd"
              ></path>
            </svg>
            <span class="text-sm">{customError}</span>
          </div>
        {/if}

        <!-- Apply Button -->
        <button class="btn btn-primary btn-sm w-full" onclick={applyCustom}>
          Apply time range
        </button>
      </div>

      <!-- Vertical separator -->
      <div class="w-px bg-base-300"></div>

      <!-- Right Section - Recently Used -->
      <div class="space-y-4">
        <div class="text-sm font-medium text-base-content">Recently used</div>
        <div class="space-y-0">
          {#if recentTimeRanges.length === 0}
            <div
              class="w-full text-left px-2 py-1 text-sm text-base-content/60"
            >
              No recent time ranges
            </div>
          {:else}
            {#each recentTimeRanges as entry, index}
              <button
                class="w-full text-left px-2 py-1 text-sm hover:bg-base-200 transition-colors flex items-center gap-2"
                onclick={() => applyRecentTimeRange(index)}
              >
                {#if index === 0 && timeContext.selection.type === 'recent'}
                  <svg
                    class="w-4 h-4 text-primary"
                    fill="currentColor"
                    viewBox="0 0 20 20"
                  >
                    <path
                      fill-rule="evenodd"
                      d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z"
                      clip-rule="evenodd"
                    ></path>
                  </svg>
                {:else}
                  <div class="w-4 h-4"></div>
                {/if}
                <span>{formatDateTimeRange(entry.start, entry.end, timezone)}</span>
                <div class="w-4 h-4"></div>
              </button>
            {/each}
          {/if}
        </div>
      </div>
    </div>

    <!-- Bottom - Timezone Selector -->
    <div class="mt-3 pt-4 border-t border-base-300">
      <button
        class="w-full flex items-center justify-between hover:bg-base-200 transition-colors rounded p-2"
        onclick={() =>
          timeContext.setTimezone(timezone === 'UTC' ? 'local' : 'UTC')}
      >
        <div class="text-sm text-base-content/80">
          {timezone === 'UTC'
            ? 'Coordinated Universal Time UTC'
            : getLocalTimezoneName()}
        </div>
        <div class="flex items-center gap-2">
          <span class="text-sm text-base-content/60">
            {timezone === 'UTC' ? 'UTC+0' : 'Local'}
          </span>
          <svg
            class="w-4 h-4 text-base-content/60"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M8 9l4-4 4 4m0 6l-4 4-4-4"
            ></path>
          </svg>
        </div>
      </button>
    </div>
  </div>
</div>

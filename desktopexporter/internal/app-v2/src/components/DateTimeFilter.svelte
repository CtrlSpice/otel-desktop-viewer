<script lang="ts">
  import * as chrono from 'chrono-node';

  export let value: { start?: string; end?: string } = { start: '', end: '' };
  export let timezone: string = 'UTC';

  let isOpen = false;
  let customStart = '';
  let customEnd = '';
  let customStartText = '';
  let customEndText = '';

  // Preset time ranges
  const presets = [
    { label: 'Show all', special: 'all' },
    { label: 'Last 5 minutes', minutes: 5 },
    { label: 'Last 15 minutes', minutes: 15 },
    { label: 'Last 30 minutes', minutes: 30 },
    { label: 'Last hour', hours: 1 },
    { label: 'Last 6 hours', hours: 6 },
    { label: 'Last day', days: 1 },
    { label: 'Last 3 days', days: 3 },
    { label: 'Last week', days: 7 },
  ];

  // Timezone options
  const timezones = [
    { value: 'UTC', label: 'UTC' },
    { value: 'local', label: 'Local' },
  ];

  // Default to last 15 minutes
  $: if (!value.start && !value.end) {
    const now = new Date();
    const fifteenMinutesAgo = new Date(now.getTime() - 15 * 60 * 1000);
    value = {
      start: fifteenMinutesAgo.toISOString(),
      end: now.toISOString(),
    };
  }

  // Get display text for current selection
  $: displayText = getDisplayText();

  function getDisplayText(): string {
    if (!value.start && !value.end) return 'Last 15 minutes';

    const start = value.start ? new Date(value.start) : null;
    const end = value.end ? new Date(value.end) : null;

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

  function applyPreset(preset: (typeof presets)[0]) {
    const now = new Date();
    let start: Date;

    if (preset.special === 'all') {
      // Dispatch change event
      const changeEvent = new CustomEvent('change', {
        detail: { start: undefined, end: undefined },
      });
      document.dispatchEvent(changeEvent);
      isOpen = false;
      return;
    }

    if (preset.minutes) {
      start = new Date(now.getTime() - preset.minutes * 60 * 1000);
    } else if (preset.hours) {
      start = new Date(now.getTime() - preset.hours * 60 * 60 * 1000);
    } else if (preset.days) {
      start = new Date(now.getTime() - preset.days * 24 * 60 * 60 * 1000);
    } else {
      return;
    }

    // Dispatch change event
    const changeEvent = new CustomEvent('change', {
      detail: {
        start: start.toISOString(),
        end: now.toISOString(),
      },
    });
    document.dispatchEvent(changeEvent);
    isOpen = false;
  }

  function applyCustom() {
    if (!customStart && !customEnd) {
      const changeEvent = new CustomEvent('change', {
        detail: { start: undefined, end: undefined },
      });
      document.dispatchEvent(changeEvent);
      isOpen = false;
      return;
    }

    const changeEvent = new CustomEvent('change', {
      detail: {
        start: customStart || undefined,
        end: customEnd || undefined,
      },
    });
    document.dispatchEvent(changeEvent);
    isOpen = false;
  }

  function handleTimezoneChange(event: Event) {
    const target = event.target as HTMLSelectElement;
    timezone = target.value;
    const timezoneEvent = new CustomEvent('timezoneChange', {
      detail: timezone,
    });
    document.dispatchEvent(timezoneEvent);
  }

  function formatDateTimeForInput(date: Date): string {
    // Format as YYYY-MM-DDTHH:MM for datetime-local input
    const year = date.getFullYear();
    const month = String(date.getMonth() + 1).padStart(2, '0');
    const day = String(date.getDate()).padStart(2, '0');
    const hours = String(date.getHours()).padStart(2, '0');
    const minutes = String(date.getMinutes()).padStart(2, '0');

    return `${year}-${month}-${day}T${hours}:${minutes}`;
  }

  function isCurrentPreset(preset: (typeof presets)[0]): boolean {
    if (preset.special === 'all') {
      return !value.start && !value.end;
    }

    if (!value.start || !value.end) return false;

    const start = new Date(value.start);
    const end = new Date(value.end);
    const now = new Date();

    if (preset.minutes) {
      const expectedStart = new Date(
        now.getTime() - preset.minutes * 60 * 1000
      );
      return Math.abs(start.getTime() - expectedStart.getTime()) < 60000; // Within 1 minute
    } else if (preset.hours) {
      const expectedStart = new Date(
        now.getTime() - preset.hours * 60 * 60 * 1000
      );
      return Math.abs(start.getTime() - expectedStart.getTime()) < 300000; // Within 5 minutes
    } else if (preset.days) {
      const expectedStart = new Date(
        now.getTime() - preset.days * 24 * 60 * 60 * 1000
      );
      return Math.abs(start.getTime() - expectedStart.getTime()) < 3600000; // Within 1 hour
    }

    return false;
  }

  function parseNaturalLanguage(text: string): string | null {
    if (!text.trim()) return null;

    try {
      const parsed = chrono.parseDate(text);
      if (parsed) {
        return parsed.toISOString();
      }
    } catch (error) {
      console.warn('Failed to parse natural language:', text, error);
    }
    return null;
  }

  function handleCustomStartChange() {
    const parsed = parseNaturalLanguage(customStartText);
    if (parsed) {
      customStart = formatDateTimeForInput(new Date(parsed));
    }
  }

  function handleCustomEndChange() {
    const parsed = parseNaturalLanguage(customEndText);
    if (parsed) {
      customEnd = formatDateTimeForInput(new Date(parsed));
    }
  }
</script>

<div class="relative">
  <!-- Dropdown Button -->
  <button
    class="input input-bordered input-sm flex items-center gap-2"
    on:click={() => (isOpen = !isOpen)}
  >
    <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
      <path
        stroke-linecap="round"
        stroke-linejoin="round"
        stroke-width="2"
        d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"
      ></path>
    </svg>
    <span>{displayText}</span>
    <svg
      class="w-3 h-3 transition-transform duration-200 {isOpen
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

  <!-- Drawer -->
  {#if isOpen}
    <div
      class="absolute top-full left-0 mt-1 w-full bg-base-100 border border-base-300 rounded shadow-lg z-50"
    >
      <div class="p-4">
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
                bind:value={customStartText}
                on:input={handleCustomStartChange}
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
                bind:value={customEndText}
                on:input={handleCustomEndChange}
              />
            </div>

            <!-- Apply Button -->
            <button
              class="btn btn-primary btn-sm w-full"
              on:click={applyCustom}
            >
              Apply time range
            </button>

            <!-- Recently Used (placeholder) -->
            <div class="text-sm text-base-content/60">
              <div class="font-medium mb-2">Recently used time ranges</div>
              <div class="text-xs text-base-content/40">
                No recent time ranges
              </div>
            </div>
          </div>

          <!-- Right Side - Preset Time Ranges -->
          <div class="w-80 space-y-4">
            <div class="text-sm font-medium text-base-content">
              Preset time ranges
            </div>

            <!-- Preset Options -->
            <div class="space-y-1">
              {#each presets as preset}
                <button
                  class="w-full text-left px-3 py-2 text-sm hover:bg-base-200 transition-colors flex items-center justify-between"
                  on:click={() => applyPreset(preset)}
                >
                  <span>{preset.label}</span>
                  {#if isCurrentPreset(preset)}
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
                  {/if}
                </button>
              {/each}
            </div>
          </div>
        </div>

        <!-- Bottom - Timezone Selector -->
        <div class="mt-6 pt-4 border-t border-base-300">
          <div class="flex items-center justify-between">
            <div class="text-sm text-base-content/80">
              {timezone === 'UTC'
                ? 'Coordinated Universal Time UTC'
                : 'Local Time'}
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
          </div>
        </div>
      </div>
    </div>
  {/if}
</div>

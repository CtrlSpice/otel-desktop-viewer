<script lang="ts">
  import type { SpanData } from '@/types/api-types';
  import FieldsPanel from './FieldsPanel.svelte';
  import EventsPanel from './EventsPanel.svelte';
  import LinksPanel from './LinksPanel.svelte';

  type Props = {
    span: SpanData | undefined;
  };

  let { span }: Props = $props();

  let numEvents = $derived(span?.events.length ?? 0);
  let numLinks = $derived(span?.links.length ?? 0);
  let activeSection = $state<'fields' | 'events' | 'links'>('fields');
</script>

{#if span}
  <div class="w-96 h-full flex flex-col">
    <!-- Button Row -->
    <div
      class="flex items-center gap-2 px-4 py-2 bg-base-200 border border-base-300 rounded-lg flex-shrink-0"
    >
      <button
        class="px-3 py-1.5 rounded-xl text-xs font-medium transition-colors flex items-center gap-1.5 justify-center flex-1
        {activeSection === 'fields'
          ? 'bg-primary text-primary-content'
          : 'text-base-content hover:bg-base-300'}"
        onclick={() => (activeSection = 'fields')}
      >
        <svg
          xmlns="http://www.w3.org/2000/svg"
          width="16"
          height="16"
          viewBox="0 0 24 24"
          class="w-4 h-4"
        >
          <g fill="none" stroke="currentColor">
            <path
              d="M2.5 12c0-4.478 0-6.718 1.391-8.109S7.521 2.5 12 2.5c4.478 0 6.718 0 8.109 1.391S21.5 7.521 21.5 12c0 4.478 0 6.718-1.391 8.109S16.479 21.5 12 21.5c-4.478 0-6.718 0-8.109-1.391S2.5 16.479 2.5 12Z"
            />
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              d="M2.5 12h19M13 7h4"
            />
            <circle
              cx="8.25"
              cy="7"
              r="1.25"
              stroke-linecap="round"
              stroke-linejoin="round"
            />
            <circle
              cx="8.25"
              cy="17"
              r="1.25"
              stroke-linecap="round"
              stroke-linejoin="round"
            />
            <path stroke-linecap="round" stroke-linejoin="round" d="M13 17h4" />
          </g>
        </svg>
        <span>Fields</span>
      </button>
      <button
        class="px-3 py-1.5 rounded-lg text-xs font-medium transition-colors flex items-center gap-1.5 justify-center flex-1 {activeSection ===
        'events'
          ? 'bg-primary text-primary-content'
          : 'text-base-content hover:bg-base-300'}"
        onclick={() => (activeSection = 'events')}
      >
        <svg
          xmlns="http://www.w3.org/2000/svg"
          width="16"
          height="16"
          viewBox="0 0 24 24"
          class="w-4 h-4"
        >
          <path
            fill="none"
            stroke="currentColor"
            stroke-linecap="round"
            stroke-linejoin="round"
            d="M18.01 7.49L19.5 6m1 7.5a8.5 8.5 0 1 1-17 0a8.5 8.5 0 0 1 17 0M14.5 2h-5M12 13.5l3.5-3.5"
          />
        </svg>
        <span>Events</span>
        <span
          class="badge badge-sm {activeSection === 'events'
            ? 'badge-primary-content'
            : 'badge-secondary badge-outline'}">{numEvents}</span
        >
      </button>
      <button
        class="px-3 py-1.5 rounded-lg text-xs font-medium transition-colors flex items-center gap-1.5 justify-center flex-1 {activeSection ===
        'links'
          ? 'bg-primary text-primary-content'
          : 'text-base-content hover:bg-base-300'}"
        onclick={() => (activeSection = 'links')}
      >
        <svg
          xmlns="http://www.w3.org/2000/svg"
          width="16"
          height="16"
          viewBox="0 0 24 24"
          class="w-4 h-4"
        >
          <g fill="none" stroke="currentColor" stroke-linecap="round">
            <path
              d="M10 13.229q.213.349.504.654a3.56 3.56 0 0 0 4.454.59q.391-.24.73-.59l3.239-3.372c1.43-1.49 1.43-3.904 0-5.394a3.564 3.564 0 0 0-5.183 0l-.714.743"
            />
            <path
              d="m10.97 18.14l-.713.743a3.564 3.564 0 0 1-5.184 0c-1.43-1.49-1.43-3.905 0-5.394l3.24-3.372a3.564 3.564 0 0 1 5.183 0q.291.305.504.654"
            />
          </g>
        </svg>
        <span>Links</span>
        <span
          class="badge badge-sm {activeSection === 'links'
            ? 'badge-primary-content'
            : 'badge-secondary badge-outline'}">{numLinks}</span
        >
      </button>
    </div>

    <!-- Content Area -->
    <div class="flex-1 overflow-y-auto pt-2">
      {#if activeSection === 'fields'}
        <FieldsPanel {span} />
      {:else if activeSection === 'events'}
        {#if numEvents > 0}
          <EventsPanel events={span.events} spanStartTime={span.startTime} />
        {:else}
          <div class="flex flex-col items-center justify-center h-full p-8">
            <svg
              class="w-12 h-12 text-base-content/30 mb-4"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                d="M18.01 7.49L19.5 6m1 7.5a8.5 8.5 0 1 1-17 0a8.5 8.5 0 0 1 17 0M14.5 2h-5M12 13.5l3.5-3.5"
              />
            </svg>
            <p class="text-base-content/60 text-sm">
              No events recorded for this span
            </p>
          </div>
        {/if}
      {:else if activeSection === 'links'}
        {#if numLinks > 0}
          <LinksPanel links={span.links} />
        {:else}
          <div class="flex flex-col items-center justify-center h-full p-8">
            <svg
              class="w-12 h-12 text-base-content/30 mb-4"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-linecap="round"
            >
              <path
                d="M10 13.229q.213.349.504.654a3.56 3.56 0 0 0 4.454.59q.391-.24.73-.59l3.239-3.372c1.43-1.49 1.43-3.904 0-5.394a3.564 3.564 0 0 0-5.183 0l-.714.743"
              />
              <path
                d="m10.97 18.14l-.713.743a3.564 3.564 0 0 1-5.184 0c-1.43-1.49-1.43-3.905 0-5.394l3.24-3.372a3.564 3.564 0 0 1 5.183 0q.291.305.504.654"
              />
            </svg>
            <p class="text-base-content/60 text-sm">
              No links recorded for this span
            </p>
          </div>
        {/if}
      {/if}
    </div>
  </div>
{:else}
  <div class="w-96 h-full overflow-y-auto pt-4">
    <p class="text-base-content/60 p-4">No span selected</p>
  </div>
{/if}

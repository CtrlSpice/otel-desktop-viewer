<script lang="ts">
  import SignalCard from '@/components/shared/SignalCard.svelte'
  import SignalDate from '@/components/shared/SignalDate.svelte'
  import {
    serviceIdentity,
    serviceSignalHref,
    type ServiceSummary,
    type DiscoveryOptions,
  } from '@/services/discovery-types'
  import { traceServiceColor } from '@/utils/trace-service-color'
  import { themeSignal } from '@/state/theme.svelte'

  let {
    service,
    selected,
    onSelect,
    range,
  }: {
    service: ServiceSummary
    selected: boolean
    onSelect: () => void
    range: Pick<DiscoveryOptions, 'startTime' | 'endTime'>
  } = $props()
  const signals = $derived([
    {
      id: 'traces' as const,
      title: 'Traces',
      count: service.spanCount,
      noun: 'spans',
      errors: service.errorSpanCount,
    },
    {
      id: 'logs' as const,
      title: 'Logs',
      count: service.logCount,
      noun: 'logs',
      errors: service.errorLogCount,
    },
    {
      id: 'metrics' as const,
      title: 'Metrics',
      count: service.metricCount,
      noun: 'metrics',
      errors: null,
    },
  ])
</script>

<SignalCard
  container
  id={serviceIdentity(service)}
  title={service.serviceName || 'Unnamed service'}
  {selected}
  wrapTitle
>
  {#snippet titleContent()}
    <button
      class="service-title"
      onclick={onSelect}
      aria-pressed={selected}
      aria-label="Inspect {service.serviceName ||
        'unnamed service'}{service.serviceNamespace
        ? ` · ${service.serviceNamespace}`
        : ''}"
    >
      <span
        class="service-dot"
        style:background-color={traceServiceColor(
          service.serviceName,
          themeSignal.value
        )}
        aria-hidden="true"
      ></span>
      <span
        >{service.serviceName ||
          'Unnamed service'}{#if service.serviceNamespace}<span
            class="text-rp-subtle"
          >
            · {service.serviceNamespace}</span
          >{/if}</span
      >
    </button>
  {/snippet}
  {#snippet meta()}
    <div class="stats service-stats">
      {#each signals as signal (signal.id)}
        <div class="stat">
          <a
            class="stat-title"
            href={serviceSignalHref(service, signal.id, range)}
            aria-label="Open {service.serviceName ||
              'unnamed service'} {signal.id}">{signal.title}</a
          >
          <div class="stat-value">
            {signal.count.toLocaleString()}
            {signal.noun}
          </div>
          <div class="stat-desc">
            {#if signal.errors !== null}
              {#if signal.errors > 0}<span class="error-dot" aria-hidden="true"
                ></span>{/if}
              {signal.errors.toLocaleString()}
              {signal.errors === 1 ? 'error' : 'errors'}
            {:else}{service.dataPointCount.toLocaleString()} pts{/if}
          </div>
        </div>
      {/each}
    </div>
  {/snippet}
  {#snippet date()}<SignalDate
      label="Last seen"
      timestamp={service.lastSeen && service.lastSeen !== '0'
        ? BigInt(service.lastSeen)
        : undefined}
    />{/snippet}
</SignalCard>

<style lang="postcss">
  @reference '../../app.css';
  .service-title {
    @apply flex min-w-0 items-center gap-2 text-left font-normal;
  }
  .service-dot,
  .error-dot {
    @apply inline-block size-2 shrink-0 rounded-full;
  }
  .error-dot {
    @apply bg-error;
  }
  .service-stats {
    @apply grid w-full min-w-0 grid-cols-3 gap-2 overflow-visible rounded-none bg-transparent py-2 shadow-none;
  }
  .service-stats .stat {
    @apply min-w-0 border-0 p-0;
  }
  .stat-title {
    @apply text-sm font-normal text-secondary no-underline;
  }
  .stat-title:hover {
    @apply underline;
  }
  .stat-value {
    @apply whitespace-normal text-sm font-normal text-base-content;
    overflow-wrap: anywhere;
  }
  .stat-desc {
    @apply flex min-h-6 items-center gap-2 whitespace-normal text-sm;
    color: var(--color-subtle);
  }
</style>

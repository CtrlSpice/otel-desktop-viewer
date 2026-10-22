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
      count: service.traceCount,
      unit: service.traceCount === 1 ? 'trace' : 'traces',
      detailCount: service.spanCount,
      detail: service.spanCount === 1 ? 'span' : 'spans',
      extraCount: service.errorSpanCount,
      extra: '',
      marker: service.errorSpanCount > 0 ? 'bg-error' : '',
    },
    {
      id: 'logs' as const,
      title: 'Logs',
      count: service.logCount,
      unit: service.logCount === 1 ? 'log' : 'logs',
      detailCount: null,
      detail: '',
      extraCount: service.errorLogCount,
      extra: '',
      marker: service.errorLogCount > 0 ? 'bg-error' : '',
    },
    {
      id: 'metrics' as const,
      title: 'Metrics',
      count: service.metricCount,
      unit: service.metricCount === 1 ? 'metric' : 'metrics',
      detailCount: service.dataPointCount,
      detail: 'pts',
      extraCount: service.exemplarCount,
      extra: service.exemplarCount === 1 ? 'exemplar' : 'exemplars',
      marker: '',
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
      class="flex min-w-0 items-center gap-2 text-left font-normal"
      onclick={onSelect}
      aria-pressed={selected}
      aria-label="Inspect {service.serviceName ||
        'unnamed service'}{service.serviceNamespace
        ? ` · ${service.serviceNamespace}`
        : ''}"
    >
      <span
        class="inline-block size-2 shrink-0 rounded-full"
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
    <div class="service-summary">
      {#each signals as signal (signal.id)}
        <a
          class="service-summary__row"
          href={serviceSignalHref(service, signal.id, range)}
          aria-label="Open {service.serviceName ||
            'unnamed service'} {signal.id}"
        >
          <span class="service-summary__label text-rp-subtle"
            >{signal.title}</span
          >
          <span class="service-summary__counts">
            <span
              >{signal.count.toLocaleString()}
              <span class="text-rp-subtle">{signal.unit}</span></span
            >
            {#if signal.detailCount !== null}
              <span class="text-rp-subtle" aria-hidden="true">·</span>
              <span
                >{signal.detailCount.toLocaleString()}
                <span class="text-rp-subtle">{signal.detail}</span></span
              >
            {/if}
            {#if signal.extraCount > 0}
              <span class="text-rp-subtle" aria-hidden="true">·</span>
              <span class="inline-flex items-center gap-2">
                {#if signal.marker}<span
                    class="size-2 shrink-0 rounded-full {signal.marker}"
                    aria-hidden="true"
                  ></span>{/if}
                <span
                  class="text-base-content"
                  aria-label={signal.id === 'traces'
                    ? `${signal.extraCount} error spans`
                    : signal.id === 'logs'
                      ? `${signal.extraCount} error or fatal logs`
                      : undefined}>{signal.extraCount.toLocaleString()}</span
                >
                {#if signal.extra}<span class="text-rp-subtle"
                    >{signal.extra}</span
                  >{/if}
              </span>
            {/if}
          </span>
        </a>
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
  .service-summary {
    @apply grid w-full min-w-0 gap-x-2 text-sm;
    grid-template-columns: max-content minmax(0, 1fr);
  }
  .service-summary__row {
    @apply col-span-2 grid items-center text-base-content no-underline;
    grid-template-columns: subgrid;
    min-height: var(--signal-card-row-h);
  }
  .service-summary__row:hover .service-summary__label {
    @apply underline;
  }
  .service-summary__counts {
    @apply flex items-center gap-2 whitespace-nowrap;
  }
</style>

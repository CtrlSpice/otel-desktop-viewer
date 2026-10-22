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
    <div class="stats service-stats">
      {#each signals as signal (signal.id)}
        <div class="stat place-items-center text-center">
          <a
            class="stat-title"
            href={serviceSignalHref(service, signal.id, range)}
            aria-label="Open {service.serviceName ||
              'unnamed service'} {signal.id}"
            >{signal.title}<span class="text-rp-subtle"
              >{signal.count.toLocaleString()}</span
            ></a
          >
          {#if signal.detailCount !== null}<div class="stat-desc">
              <span class="text-base-content"
                >{signal.detailCount.toLocaleString()}</span
              >
              {signal.detail}
            </div>{/if}
          {#if signal.extraCount > 0}<div class="stat-desc stat-desc--extra">
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
              {signal.extra}
            </div>{/if}
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
  .service-stats {
    @apply grid w-full min-w-0 grid-cols-3 gap-x-2 overflow-visible rounded-none bg-transparent shadow-none;
  }
  .service-stats .stat {
    @apply grid min-w-0 items-center border-0 p-0;
    grid-row: span 3;
    grid-template-rows: subgrid;
  }
  .stat-title {
    grid-row: 1;
    @apply flex flex-wrap items-center justify-center gap-1 self-start text-sm font-normal text-secondary no-underline;
  }
  .stat-title:hover {
    @apply underline;
  }
  .stat-desc {
    grid-row: 2;
    @apply flex items-center gap-2 whitespace-normal text-sm;
    color: var(--color-subtle);
  }
  .stat-desc--extra {
    grid-row: 3;
  }
  .stat-title,
  .stat-desc {
    min-height: var(--signal-card-row-h);
  }
</style>

<script lang="ts">
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import {
    BarChartHorizontalIcon,
    ChartHistogramIcon,
  } from '@hugeicons/core-free-icons'
  import { LogIcon } from '@/icons'
  import SignalCard from '@/components/shared/SignalCard.svelte'
  import SignalDate from '@/components/shared/SignalDate.svelte'
  import GraphColorPicker from '@/components/shared/GraphColorPicker.svelte'
  import {
    SIGNAL_ORDER,
    serviceIdentity,
    type ServiceSummary,
  } from '@/services/discovery-types'
  import { traceServiceColor } from '@/utils/trace-service-color'
  import { themeSignal } from '@/state/theme.svelte'

  let {
    service,
    selected,
    onSelect,
  }: {
    service: ServiceSummary
    selected: boolean
    onSelect: () => void
  } = $props()
  const serviceColor = $derived(
    traceServiceColor(service.serviceName, themeSignal.value)
  )
  const signals = $derived.by(() => {
    const bySignal = {
      traces: {
        id: 'traces' as const,
        title: 'Traces',
        count: service.traceCount,
        detailCount: service.spanCount,
        detail: service.spanCount === 1 ? 'span' : 'spans',
        extraCount: service.errorSpanCount,
        extra: 'err',
        marker: service.errorSpanCount > 0 ? 'bg-error' : '',
      },
      logs: {
        id: 'logs' as const,
        title: 'Logs',
        count: service.logCount,
        detailCount: null,
        detail: '',
        extraCount: service.errorLogCount,
        extra: 'err',
        marker: service.errorLogCount > 0 ? 'bg-error' : '',
      },
      metrics: {
        id: 'metrics' as const,
        title: 'Metrics',
        count: service.metricCount,
        detailCount: service.dataPointCount,
        detail: 'pts',
        extraCount: service.exemplarCount,
        extra: service.exemplarCount === 1 ? 'exemplar' : 'exemplars',
        marker: '',
      },
    }
    return SIGNAL_ORDER.map(signal => bySignal[signal])
  })
</script>

<SignalCard
  id={serviceIdentity(service)}
  title={service.serviceName || 'Unnamed service'}
  {selected}
  onclick={onSelect}
  wrapTitle
>
  {#snippet action()}
    <GraphColorPicker
      scope="services"
      identity={service.serviceName}
      label={`Colour for ${service.serviceName || 'Unnamed service'}`}
      color={serviceColor}
      compactTrigger
    />
  {/snippet}
  {#snippet titleContent()}
    <span class="flex min-w-0 items-center gap-2 text-left font-normal">
      <span
        >{service.serviceName ||
          'Unnamed service'}{#if service.serviceNameKind && service.serviceNameKind !== 'string'}<span
            class="text-secondary text-sm"
          >
            · {service.serviceNameKind}</span
          >{/if}{#if service.serviceNamespace || (service.serviceNamespaceKind && service.serviceNamespaceKind !== 'string')}<span
            class="text-subtle"
          >
            · {service.serviceNamespace}{#if service.serviceNamespaceKind && service.serviceNamespaceKind !== 'string'}<span
                class="text-secondary text-sm"
              >
                · {service.serviceNamespaceKind}</span
              >{/if}</span
          >{/if}</span
      >
    </span>
  {/snippet}
  {#snippet meta()}
    <div class="service-summary">
      {#each signals as signal (signal.id)}
        <div class="service-summary__row">
          <span
            class="service-summary__label inline-flex items-center text-subtle"
          >
            <span class="service-summary__icon">
              {#if signal.id === 'logs'}
                <LogIcon class="size-4 shrink-0" aria-hidden="true" />
              {:else}
                <HugeiconsIcon
                  icon={signal.id === 'traces'
                    ? BarChartHorizontalIcon
                    : ChartHistogramIcon}
                  size={16}
                  strokeWidth={1.5}
                  class="shrink-0"
                  aria-hidden="true"
                />
              {/if}
            </span>
            <span>{signal.title}:</span>
          </span>
          <span class="service-summary__counts">
            <span>{signal.count.toLocaleString()}</span>
            {#if signal.detailCount !== null}
              <span class="text-subtle" aria-hidden="true">·</span>
              <span
                >{signal.detailCount.toLocaleString()}
                <span class="text-subtle">{signal.detail}</span></span
              >
            {/if}
            {#if signal.extraCount > 0}
              {#if !signal.marker}<span class="text-subtle" aria-hidden="true"
                  >·</span
                >{/if}
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
                {#if signal.extra}<span class="text-subtle">{signal.extra}</span
                  >{/if}
              </span>
            {/if}
          </span>
        </div>
      {/each}
    </div>
  {/snippet}
  {#snippet date()}
    <div class="pl-1">
      <SignalDate
        label="Last seen"
        timestamp={service.lastSeen && service.lastSeen !== '0'
          ? BigInt(service.lastSeen)
          : undefined}
      />
    </div>
  {/snippet}
</SignalCard>

<style lang="postcss">
  @reference '../../app.css';
  .service-summary {
    @apply grid w-full min-w-0 gap-x-2 text-sm;
    grid-template-columns: max-content minmax(0, 1fr);
  }
  .service-summary__row {
    @apply col-span-2 grid items-center text-base-content;
    grid-template-columns: subgrid;
    min-height: var(--signal-card-row-h);
  }
  .service-summary__counts {
    @apply flex items-center gap-2 whitespace-nowrap;
  }
  .service-summary__icon {
    @apply inline-flex w-6 shrink-0 items-center justify-center;
  }
</style>

<script lang="ts">
  import SignalCard from '@/components/shared/SignalCard.svelte'
  import SignalDate from '@/components/shared/SignalDate.svelte'
  import GraphColorPicker from '@/components/shared/GraphColorPicker.svelte'
  import {
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
  const errorCount = $derived(service.errorSpanCount + service.errorLogCount)
  const errorLabel = $derived(
    `${service.errorSpanCount.toLocaleString()} error spans · ${service.errorLogCount.toLocaleString()} error or fatal logs`
  )
</script>

<SignalCard
  id={serviceIdentity(service)}
  title={service.serviceName || 'Unnamed service'}
  {selected}
  onclick={onSelect}
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
  {#snippet lead()}
    <span class="badge-count pl-6"
      >{service.traceCount.toLocaleString()}
      {service.traceCount === 1 ? 'trace' : 'traces'}, {service.metricCount.toLocaleString()}
      {service.metricCount === 1 ? 'metric' : 'metrics'}, {service.logCount.toLocaleString()}
      {service.logCount === 1 ? 'log' : 'logs'}</span
    >
  {/snippet}
  {#snippet date()}
    <div class="pl-6">
      <SignalDate
        label="Last seen"
        timestamp={service.lastSeen && service.lastSeen !== '0'
          ? BigInt(service.lastSeen)
          : undefined}
      />
    </div>
  {/snippet}
  {#snippet badge()}
    {#if errorCount > 0}<span
        class="inline-flex items-center gap-2"
        title={errorLabel}
        aria-label={errorLabel}
      >
        <span class="size-2 shrink-0 rounded-full bg-error" aria-hidden="true"
        ></span>
        <span class="badge-count">[{errorCount.toLocaleString()} err]</span>
      </span>{/if}
  {/snippet}
</SignalCard>

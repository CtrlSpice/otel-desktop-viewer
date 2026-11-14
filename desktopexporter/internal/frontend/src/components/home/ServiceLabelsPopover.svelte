<script lang="ts">
  import { createLoadingNotice } from '@/components/shared/utils/loading-notice.svelte'
  import { createRead } from '@/services/reads'
  import DetailProperty from '@/components/shared/DetailProperty.svelte'
  import {
    createPopoverID,
    setupAnchorPopover,
  } from '@/components/shared/utils/anchor-popover'
  import { telemetryAPI } from '@/services/telemetry-service'
  import {
    serviceIdentity,
    type ServiceSummary,
  } from '@/services/discovery-types'
  let { service }: { service: ServiceSummary } = $props()
  const read = createRead<{
    serviceName: string
    serviceNamespace: string
  }>({ formatError: String })
  const labels = $derived(read.data)
  const loading = $derived(read.pending)
  const loadingNotice = createLoadingNotice(
    () => loading,
    () => read.requestKey
  )
  const error = $derived(read.error ?? '')
  const id = createPopoverID('service-labels')
  let trigger = $state<HTMLButtonElement>()
  let popover = $state<HTMLDivElement>()
  let open = $state(false)
  $effect(() => {
    if (!trigger || !popover) return
    return setupAnchorPopover({
      trigger,
      popover,
      anchor: 'below-end',
      onOpenChange: next => (open = next),
    })
  })
  $effect(() => {
    void service
    read.clear()
    return () => read.cancel()
  })
  async function inspect() {
    await read.load(signal =>
      telemetryAPI.getServiceLabels(serviceIdentity(service), signal)
    )
  }
</script>

<button
  type="button"
  class="btn btn-ghost btn-sm font-normal"
  popovertarget={id}
  aria-expanded={open}
  bind:this={trigger}>Service labels</button
>
<div
  {id}
  bind:this={popover}
  popover="auto"
  class="anchor-popover anchor-popover--anchored service-labels"
  role="group"
  aria-label="Service labels"
>
  {#if open}
    {#if service.labelsComplete === false && !labels}
      <button
        type="button"
        class="btn btn-ghost btn-sm font-normal"
        disabled={loading}
        onclick={inspect}
        >{loadingNotice.visible
          ? 'Loading labels…'
          : 'Show full labels'}</button
      >
    {/if}
    {#if error}<p role="alert" class="text-papaya">{error}</p>{/if}
    <table class="detail-fields w-full" aria-label="Service labels">
      <tbody>
        <DetailProperty
          fieldName="service.name"
          fieldType={service.serviceNameKind ?? 'string'}
          fieldValue={labels?.serviceName ?? service.serviceName}
          sourceKey={service}
          loadText={async signal =>
            (
              await telemetryAPI.getServiceLabels(
                serviceIdentity(service),
                signal
              )
            ).serviceName}
        />
        <DetailProperty
          fieldName="service.namespace"
          fieldType={service.serviceNamespaceKind ?? 'string'}
          fieldValue={labels?.serviceNamespace ?? service.serviceNamespace}
          sourceKey={service}
          loadText={async signal =>
            (
              await telemetryAPI.getServiceLabels(
                serviceIdentity(service),
                signal
              )
            ).serviceNamespace}
        />
      </tbody>
    </table>
  {/if}
</div>

<style>
  .service-labels {
    width: 32rem;
    max-width: calc(100vw - 1.5rem);
    max-height: min(85vh, calc(100vh - 2rem));
    overflow-y: auto;
  }
</style>

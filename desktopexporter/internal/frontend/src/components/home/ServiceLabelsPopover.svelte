<script lang="ts">
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
  let labels = $state<{ serviceName: string; serviceNamespace: string }>()
  let loading = $state(false)
  let error = $state('')
  let request: AbortController | undefined
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
    labels = undefined
    error = ''
    loading = false
    return () => request?.abort()
  })
  async function inspect() {
    request?.abort()
    const controller = new AbortController()
    request = controller
    loading = true
    error = ''
    try {
      const result = await telemetryAPI.getServiceLabels(
        serviceIdentity(service),
        controller.signal
      )
      if (!controller.signal.aborted) labels = result
    } catch (reason) {
      if (!controller.signal.aborted) error = String(reason)
    } finally {
      if (!controller.signal.aborted) loading = false
    }
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
        >{loading ? 'Loading labels…' : 'Show full labels'}</button
      >
    {/if}
    {#if error}<p role="alert" class="text-papaya">{error}</p>{/if}
    <table class="detail-fields w-full" aria-label="Service labels">
      <tbody>
        <DetailProperty
          fieldName="service.name"
          fieldType={service.serviceNameKind ?? 'string'}
          fieldValue={labels?.serviceName ?? service.serviceName}
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

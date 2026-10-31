<script lang="ts">
  import { onDestroy } from 'svelte'
  import { Download01Icon } from '@hugeicons/core-free-icons'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { downloadOTLP, type ExportSignal } from '@/services/export-service'

  let { signal, id }: { signal: ExportSignal; id: string } = $props()
  let pending = $state(false)
  let error = $state('')
  let activeRequest: AbortController | undefined

  $effect(() => {
    void signal
    void id
    activeRequest?.abort()
    activeRequest = undefined
    pending = false
    error = ''
  })
  onDestroy(() => activeRequest?.abort())

  async function download() {
    if (pending) return
    error = ''
    pending = true
    const controller = new AbortController()
    activeRequest = controller
    try {
      await downloadOTLP(signal, id, controller.signal)
    } catch (err) {
      if (!controller.signal.aborted) {
        error = err instanceof Error ? err.message : 'Download failed'
      }
    } finally {
      if (activeRequest === controller) {
        pending = false
        activeRequest = undefined
      }
    }
  }
</script>

<button
  type="button"
  class="header-icon-button tooltip tooltip-left"
  onclick={download}
  aria-label="Download {signal}"
  data-tip="Download {signal}"
  aria-busy={pending}
  aria-disabled={pending}
  disabled={!id}
>
  <HugeiconsIcon
    icon={Download01Icon}
    size={16}
    strokeWidth={1.5}
    aria-hidden="true"
  />
</button>

{#if error}
  <span role="alert" class="text-error text-xs">{error}</span>
{/if}

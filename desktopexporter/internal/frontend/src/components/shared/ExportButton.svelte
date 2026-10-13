<script lang="ts">
  import { onDestroy } from 'svelte'
  import { FileExportIcon } from '@hugeicons/core-free-icons'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { downloadOTLP, type ExportSignal } from '@/services/export-service'

  let { signal, id }: { signal: ExportSignal; id: string } = $props()
  let pending = $state(false)
  let error = $state('')
  let activeRequest: AbortController | undefined

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
        error = err instanceof Error ? err.message : 'Export failed'
      }
    } finally {
      pending = false
      activeRequest = undefined
    }
  }
</script>

<button
  type="button"
  class="btn btn-circle btn-primary btn-soft btn-xs tooltip tooltip-right"
  class:btn-disabled={pending}
  onclick={download}
  aria-label="Export {signal}"
  aria-busy={pending}
  aria-disabled={pending}
  data-tip={`Export ${signal}`}
  disabled={!id}
>
  <HugeiconsIcon
    icon={FileExportIcon}
    size="1em"
    strokeWidth={1.5}
    aria-hidden="true"
  />
</button>

{#if error}
  <span role="alert" class="text-error text-xs">{error}</span>
{/if}

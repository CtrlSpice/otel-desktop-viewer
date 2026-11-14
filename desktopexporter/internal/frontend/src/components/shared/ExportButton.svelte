<script lang="ts">
  import { createRead } from '@/services/reads'
  import { Download01Icon } from '@hugeicons/core-free-icons'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import IconButton from './IconButton.svelte'
  import { downloadOTLP, type ExportSignal } from '@/services/export-service'

  let { signal, id }: { signal: ExportSignal; id: string } = $props()
  const downloadRead = createRead<void>({
    group: null,
    errorMessage: 'Download failed',
  })
  const pending = $derived(downloadRead.pending)
  const error = $derived(downloadRead.error ?? '')

  $effect(() => {
    void signal
    void id
    downloadRead.clear()
  })

  async function download() {
    if (pending) return
    await downloadRead.load(abort => downloadOTLP(signal, id, abort))
  }
</script>

<IconButton
  onclick={download}
  label="Download {signal}"
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
</IconButton>

{#if error}
  <span role="alert" class="text-error text-xs">{error}</span>
{/if}

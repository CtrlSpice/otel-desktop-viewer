<script lang="ts">
  import { FileImportIcon } from '@hugeicons/core-free-icons'
  import { HugeiconsIcon } from '@hugeicons/svelte'

  let { onFiles }: { onFiles: (files: File[]) => void } = $props()
  let input = $state<HTMLInputElement | null>(null)

  function chooseFiles(event: Event) {
    const field = event.currentTarget as HTMLInputElement
    const files = Array.from(field.files ?? [])
    field.value = ''
    if (files.length > 0) onFiles(files)
  }
</script>

<button
  type="button"
  class="item-state relative flex min-h-28 w-full cursor-pointer flex-col items-center justify-center rounded-xl border border-dashed border-primary px-8 py-6 text-center text-sm"
  aria-label="Import files"
  onclick={() => input?.click()}
>
  <span class="absolute top-2 right-2 text-base-content">
    <HugeiconsIcon
      icon={FileImportIcon}
      size={16}
      strokeWidth={1.5}
      aria-hidden="true"
    />
  </span>
  <span class="text-rp-subtle">
    Drop telemetry or evaluation JSON / JSONL files anywhere on the screen. I'm
    not a cop.
  </span>
</button>
<input
  bind:this={input}
  type="file"
  accept=".json,.jsonl,application/json,application/x-ndjson"
  multiple
  hidden
  aria-label="Choose telemetry or evaluation JSON / JSONL files"
  onchange={chooseFiles}
/>

<script lang="ts">
  import { FileImportIcon } from '@hugeicons/core-free-icons'
  import { HugeiconsIcon } from '@hugeicons/svelte'

  let { onFiles }: { onFiles: (files: File[]) => void } = $props()
  let input = $state<HTMLInputElement | null>(null)

  function chooseFiles(event: Event) {
    const field = event.currentTarget
    if (!(field instanceof HTMLInputElement)) return
    const files = Array.from(field.files ?? [])
    field.value = ''
    if (files.length > 0) onFiles(files)
  }
</script>

<button
  type="button"
  class="item-state flex min-h-20 w-full cursor-pointer items-center gap-3 rounded-xl border border-dashed border-primary p-3 text-left text-sm"
  aria-label="Import files"
  onclick={() => input?.click()}
>
  <span
    class="flex size-7 shrink-0 items-center justify-center text-base-content"
  >
    <HugeiconsIcon
      icon={FileImportIcon}
      size={16}
      strokeWidth={1.5}
      aria-hidden="true"
    />
  </span>
  <span class="text-balance text-subtle">
    Drop telemetry JSON files here or anywhere on the screen.
  </span>
</button>
<input
  bind:this={input}
  type="file"
  accept=".json,.jsonl,application/json,application/x-ndjson"
  multiple
  hidden
  aria-label="Choose telemetry JSON files"
  onchange={chooseFiles}
/>

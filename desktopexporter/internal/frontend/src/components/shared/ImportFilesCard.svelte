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
  <span
    class="absolute top-2 right-3 flex size-7 items-center justify-center text-base-content"
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

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
  class="item-state flex min-h-20 w-full flex-1 cursor-pointer flex-col items-center justify-center gap-2 rounded-xl border-2 border-dotted border-(--color-highlight-high) p-3 text-center text-sm"
  aria-label="Import files"
  onclick={() => input?.click()}
>
  <HugeiconsIcon
    icon={FileImportIcon}
    size={20}
    strokeWidth={1.5}
    class="shrink-0 text-base-content"
    aria-hidden="true"
  />
  <span class="max-w-[32ch] text-balance text-subtle">
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

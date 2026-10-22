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

<section
  class="flex flex-col items-center gap-2 px-3 py-4 text-center text-sm"
  aria-label="Import telemetry from files"
>
  <h2 class="text-lg font-normal text-base-content">
    Import telemetry from files
  </h2>
  <p class="text-rp-subtle">
    Drop telemetry or evaluation JSON / JSONL files anywhere on the screen. I'm
    not a cop.
  </p>
  <button
    type="button"
    class="header-icon-button tooltip tooltip-bottom"
    aria-label="Import files"
    data-tip="Import files"
    onclick={() => input?.click()}
  >
    <HugeiconsIcon
      icon={FileImportIcon}
      size={16}
      strokeWidth={1.5}
      aria-hidden="true"
    />
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
</section>

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
  class="card card-dash mb-6 border-primary/40 bg-base-100"
  aria-label="Import telemetry from files"
>
  <div class="card-body gap-4">
    <div class="min-w-0">
      <h2 class="card-title text-base text-base-content">
        Import telemetry from files
      </h2>
      <p class="import-copy mt-1 text-sm leading-relaxed">
        Select OTLP JSON files, or drop them literally anywhere on the screen.
        I'm not a cop.
      </p>
    </div>
    <div class="card-actions">
      <button
        type="button"
        class="btn btn-soft btn-primary btn-sm"
        style="--btn-color: var(--home-accent-text, var(--color-primary))"
        onclick={() => input?.click()}
      >
        <HugeiconsIcon
          icon={FileImportIcon}
          size={18}
          strokeWidth={1.5}
          aria-hidden="true"
        />
        Import files
      </button>
      <input
        bind:this={input}
        type="file"
        accept=".json,.jsonl,application/json,application/x-ndjson"
        multiple
        hidden
        aria-label="Choose OTLP JSON files"
        onchange={chooseFiles}
      />
    </div>
  </div>
</section>

<style>
  .import-copy {
    color: var(--home-secondary-text, var(--color-base-content));
  }
</style>

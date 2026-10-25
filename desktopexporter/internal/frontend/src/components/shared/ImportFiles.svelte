<script lang="ts">
  let { onFiles }: { onFiles: (files: File[]) => void } = $props()
  const id = $props.id()
  let input = $state<HTMLInputElement | null>(null)

  function chooseFiles(event: Event) {
    const field = event.currentTarget
    if (!(field instanceof HTMLInputElement)) return
    const files = Array.from(field.files ?? [])
    field.value = ''
    if (files.length > 0) onFiles(files)
  }
</script>

<p class="shrink-0 px-2 py-1 text-center text-sm text-subtle">
  <span id={`${id}-hint`}>Drop JSON files anywhere</span>, or
  <button
    type="button"
    class="link cursor-pointer text-primary"
    aria-label="Browse files to import"
    aria-describedby={`${id}-hint`}
    onclick={() => input?.click()}>browse</button
  >.
</p>
<input
  bind:this={input}
  type="file"
  accept=".json,.jsonl,application/json,application/x-ndjson"
  multiple
  hidden
  aria-label="Choose telemetry JSON files"
  onchange={chooseFiles}
/>

<script lang="ts">
  import { FileImportIcon } from '@hugeicons/core-free-icons'
  import { HugeiconsIcon } from '@hugeicons/svelte'

  let { onFiles }: { onFiles: (files: File[]) => void } = $props()
  let dragging = $state(false)
  let dragDepth = 0

  function isFileDrag(event: DragEvent): boolean {
    return event.dataTransfer?.types.includes('Files') ?? false
  }

  function reset() {
    dragDepth = 0
    dragging = false
  }

  function dragEnter(event: DragEvent) {
    if (!isFileDrag(event)) return
    event.preventDefault()
    dragDepth += 1
    dragging = true
  }

  function dragLeave() {
    dragDepth = Math.max(0, dragDepth - 1)
    if (dragDepth === 0) dragging = false
  }

  function dragOver(event: DragEvent) {
    if (!isFileDrag(event)) return
    event.preventDefault()
    if (event.dataTransfer) event.dataTransfer.dropEffect = 'copy'
  }

  function drop(event: DragEvent) {
    if (!isFileDrag(event)) return
    event.preventDefault()
    reset()
    const files = Array.from(event.dataTransfer?.files ?? [])
    if (files.length > 0) onFiles(files)
  }
</script>

<svelte:window
  ondragenter={dragEnter}
  ondragleave={dragLeave}
  ondragover={dragOver}
  ondrop={drop}
  onblur={reset}
/>

{#if dragging}
  <div
    class="pointer-events-none fixed inset-0 z-50 bg-base-100/75 p-4 backdrop-blur-sm sm:p-8"
    role="status"
    aria-live="polite"
  >
    <div
      class="card card-dash h-full border-primary/60 bg-base-100/30 shadow-xl"
    >
      <div class="card-body items-center justify-center gap-3 text-center">
        <div class="rounded-full bg-primary/10 p-3 text-primary">
          <HugeiconsIcon
            icon={FileImportIcon}
            size={24}
            strokeWidth={1.5}
            aria-hidden="true"
          />
        </div>
        <p class="grow-0 text-base font-semibold text-base-content">
          Drop OTLP JSON files to import
        </p>
      </div>
    </div>
  </div>
{/if}

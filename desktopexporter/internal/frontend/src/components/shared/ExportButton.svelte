<script lang="ts">
  import { onDestroy } from 'svelte'
  import { FileExportIcon } from '@hugeicons/core-free-icons'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import {
    downloadOTLP,
    type ExportFormat,
    type ExportSignal,
  } from '@/services/export-service'
  import { createPopoverID, setupAnchorPopover } from './utils/anchor-popover'

  let { signal, id }: { signal: ExportSignal; id: string } = $props()
  let trigger = $state<HTMLButtonElement | null>(null)
  let popover = $state<HTMLDivElement | null>(null)
  let open = $state(false)
  let pending = $state(false)
  let error = $state('')
  let activeRequest: AbortController | undefined
  const popoverID = createPopoverID('export-popover')

  onDestroy(() => activeRequest?.abort())

  $effect(() => {
    const element = popover
    const button = trigger
    if (!element || !button) return
    return setupAnchorPopover({
      popover: element,
      trigger: button,
      anchor: 'below-end',
      onOpenChange: next => {
        open = next
        if (next) element.querySelector<HTMLButtonElement>('button')?.focus()
      },
    })
  })

  async function download(format: ExportFormat) {
    if (pending) return
    popover?.hidePopover()
    error = ''
    pending = true
    const controller = new AbortController()
    activeRequest = controller
    try {
      await downloadOTLP(signal, id, format, controller.signal)
    } catch (err) {
      if (!controller.signal.aborted) {
        error = err instanceof Error ? err.message : 'Export failed'
      }
    } finally {
      pending = false
      activeRequest = undefined
    }
  }

  function menuKeydown(event: KeyboardEvent) {
    const buttons = Array.from(
      popover?.querySelectorAll<HTMLButtonElement>('button') ?? []
    )
    if (buttons.length === 0) return
    const current = buttons.findIndex(
      button => button === document.activeElement
    )
    let next: number
    switch (event.key) {
      case 'ArrowDown':
        next = (current + 1) % buttons.length
        break
      case 'ArrowUp':
        next = (current - 1 + buttons.length) % buttons.length
        break
      case 'Home':
        next = 0
        break
      case 'End':
        next = buttons.length - 1
        break
      case 'Escape':
        event.preventDefault()
        popover?.hidePopover()
        trigger?.focus()
        return
      case 'Tab':
        popover?.hidePopover()
        return
      default:
        return
    }
    event.preventDefault()
    buttons[next]?.focus()
  }
</script>

<!-- Keep the pending trigger focusable when the menu returns focus to it. -->
<button
  bind:this={trigger}
  type="button"
  class="btn btn-circle btn-primary btn-soft btn-xs tooltip tooltip-left"
  class:btn-disabled={pending}
  popovertarget={pending ? undefined : popoverID}
  aria-controls={popoverID}
  aria-haspopup="menu"
  aria-expanded={open}
  aria-label="Export {signal}"
  aria-busy={pending}
  aria-disabled={pending}
  data-tip={open ? '' : `Export ${signal}`}
  disabled={!id}
>
  <HugeiconsIcon
    icon={FileExportIcon}
    size="1em"
    strokeWidth={1.5}
    aria-hidden="true"
  />
</button>

<div
  bind:this={popover}
  id={popoverID}
  popover="auto"
  class="anchor-popover anchor-popover--anchored anchor-popover--menu"
  role="menu"
  aria-label="Export {signal} format"
  tabindex="-1"
  onkeydown={menuKeydown}
>
  <div class="anchor-popover-menu">
    <button
      type="button"
      role="menuitem"
      class="anchor-popover-menu__option"
      onclick={() => download('json')}>OTLP JSON</button
    >
    <button
      type="button"
      role="menuitem"
      class="anchor-popover-menu__option"
      onclick={() => download('protobuf')}>OTLP protobuf</button
    >
  </div>
</div>

{#if error}
  <span role="alert" class="text-error text-xs">{error}</span>
{/if}

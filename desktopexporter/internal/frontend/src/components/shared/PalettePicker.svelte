<script lang="ts">
  import { onMount } from 'svelte'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { PaintBrush01Icon } from '@hugeicons/core-free-icons'
  import { createPopoverID, setupAnchorPopover } from './utils/anchor-popover'

  const palettes = [
    { id: 'spring', name: 'Spring' },
    { id: 'summer', name: 'Summer' },
    { id: 'autumn', name: 'Autumn' },
    { id: 'winter', name: 'Winter' },
  ] as const
  type Palette = (typeof palettes)[number]['id']

  const popoverID = createPopoverID('theme-palette')
  let triggerEl = $state<HTMLButtonElement | null>(null)
  let popoverEl = $state<HTMLDivElement | null>(null)
  let open = $state(false)
  let selected = $state<Palette>('winter')
  const label = $derived(
    palettes.find(palette => palette.id === selected)!.name
  )

  function apply(palette: Palette) {
    selected = palette
    document.documentElement.dataset.palette = palette
    localStorage.setItem('theme-palette', palette)
  }

  onMount(() => {
    const saved = localStorage.getItem('theme-palette')
    apply(palettes.find(palette => palette.id === saved)?.id ?? 'winter')
  })

  $effect(() => {
    if (!triggerEl || !popoverEl) return
    return setupAnchorPopover({
      trigger: triggerEl,
      popover: popoverEl,
      anchor: 'below-end',
      onOpenChange: next => {
        open = next
      },
    })
  })

  function choose(palette: Palette) {
    apply(palette)
    popoverEl?.hidePopover()
    triggerEl?.focus()
  }
</script>

<button
  bind:this={triggerEl}
  type="button"
  popovertarget={popoverID}
  aria-expanded={open}
  aria-label={`Colour palette: ${label}`}
  data-tip={`Colour palette: ${label}`}
  class="btn btn-circle btn-ghost btn-sm shrink-0 text-sm text-base-content tooltip tooltip-bottom"
>
  <HugeiconsIcon
    icon={PaintBrush01Icon}
    size={17}
    strokeWidth={1.5}
    aria-hidden="true"
  />
</button>
<div
  bind:this={popoverEl}
  id={popoverID}
  popover="auto"
  class="anchor-popover anchor-popover--anchored w-44 p-2"
>
  <fieldset>
    <legend class="px-2 py-1 text-xs text-rp-subtle">Colour palette</legend>
    {#each palettes as palette}
      <label
        class="item-state flex cursor-pointer items-center gap-2 rounded-md px-2 py-2 text-sm"
      >
        <input
          type="radio"
          name={popoverID}
          class="radio radio-xs"
          checked={selected === palette.id}
          onchange={() => choose(palette.id)}
        />
        {palette.name}
      </label>
    {/each}
  </fieldset>
</div>

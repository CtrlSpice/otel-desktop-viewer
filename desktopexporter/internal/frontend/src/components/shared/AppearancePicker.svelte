<script lang="ts">
  import { onMount } from 'svelte'
  import { fade } from 'svelte/transition'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import {
    FallingStarIcon,
    FlowerIcon,
    Leaf01Icon,
    Moon02Icon,
    SnowflakeIcon,
    Sun03Icon,
  } from '@hugeicons/core-free-icons'
  import { createPopoverID, setupAnchorPopover } from './utils/anchor-popover'

  const palettes = [
    { id: 'spring', name: 'Spring', icon: FlowerIcon },
    { id: 'summer', name: 'Summer', icon: Sun03Icon },
    { id: 'autumn', name: 'Autumn', icon: Leaf01Icon },
    { id: 'winter', name: 'Winter', icon: SnowflakeIcon },
  ] as const
  type Palette = (typeof palettes)[number]['id']
  const modes = [
    { id: 'day', name: 'Light' },
    { id: 'night', name: 'Dark' },
  ] as const
  type Mode = (typeof modes)[number]['id']
  const originals = [
    { id: 'rose-pine-dawn', name: 'Dawn', mode: 'day', icon: Sun03Icon },
    { id: 'rose-pine-moon', name: 'Moon', mode: 'night', icon: Moon02Icon },
    { id: 'rose-pine', name: 'Pine', mode: 'night', icon: FallingStarIcon },
  ] as const
  type Original = (typeof originals)[number]

  const popoverID = createPopoverID('appearance')
  let triggerEl = $state<HTMLButtonElement | null>(null)
  let popoverEl = $state<HTMLDivElement | null>(null)
  let open = $state(false)
  let selected = $state<Palette | Original['id']>('winter')
  let mode = $state<Mode>('night')
  const current = $derived(
    palettes.find(palette => palette.id === selected) ??
      originals.find(theme => theme.id === selected)!
  )
  const label = $derived(`${current.name} ${mode === 'day' ? 'Light' : 'Dark'}`)

  function apply(palette: Palette, nextMode: Mode) {
    selected = palette
    mode = nextMode
    document.documentElement.dataset.palette = palette
    document.documentElement.dataset.theme = `madoromi-${nextMode}`
    localStorage.setItem('theme-palette', palette)
    localStorage.setItem('theme', `madoromi-${nextMode}`)
  }

  function applyOriginal(theme: Original) {
    selected = theme.id
    mode = theme.mode
    document.documentElement.dataset.theme = theme.id
    document.documentElement.removeAttribute('data-palette')
    localStorage.setItem('theme', theme.id)
    localStorage.removeItem('theme-palette')
  }

  onMount(() => {
    const saved = localStorage.getItem('theme-palette')
    const theme = localStorage.getItem('theme')
    const original = originals.find(option => option.id === theme)
    if (original) {
      applyOriginal(original)
      return
    }
    const savedMode =
      theme === 'madoromi-day'
        ? 'day'
        : theme === 'madoromi-night'
          ? 'night'
          : window.matchMedia('(prefers-color-scheme: dark)').matches
            ? 'night'
            : 'day'
    apply(
      palettes.find(palette => palette.id === saved)?.id ?? 'winter',
      savedMode
    )
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

  function choose(palette: Palette, nextMode: Mode) {
    apply(palette, nextMode)
    close()
  }

  function close() {
    popoverEl?.hidePopover()
    triggerEl?.focus()
  }
</script>

<button
  bind:this={triggerEl}
  type="button"
  popovertarget={popoverID}
  aria-expanded={open}
  aria-label={`Appearance: ${label}`}
  data-tip={`Appearance: ${label}`}
  class="btn btn-circle btn-ghost btn-sm shrink-0 text-sm text-base-content tooltip tooltip-bottom"
>
  <span class="appearance-picker__icon">
    {#key label}
      <span class="appearance-picker__fade" transition:fade={{ duration: 150 }}>
        <HugeiconsIcon
          icon={current.icon}
          size={17}
          strokeWidth={1.5}
          aria-hidden="true"
        />
      </span>
    {/key}
  </span>
</button>
<div
  bind:this={popoverEl}
  id={popoverID}
  popover="auto"
  class="anchor-popover anchor-popover--anchored w-72 p-2"
>
  <fieldset>
    <legend class="px-2 py-1 text-xs text-rp-subtle">Appearance</legend>
    {#each palettes as palette}
      <div class="appearance-picker__row">
        <span class="flex items-center gap-2 text-sm">
          <HugeiconsIcon
            icon={palette.icon}
            size={16}
            strokeWidth={1.5}
            aria-hidden="true"
          />
          {palette.name}
        </span>
        {#each modes as option}
          <label class="appearance-picker__choice">
            <input
              type="radio"
              name={popoverID}
              class="radio radio-xs"
              aria-label={`${palette.name} ${option.name}`}
              checked={selected === palette.id && mode === option.id}
              onchange={() => choose(palette.id, option.id)}
            />
            {option.name}
          </label>
        {/each}
      </div>
    {/each}
    <div
      class="mx-2 mt-2 border-t border-base-300 pt-2 pb-1 text-xs text-rp-subtle"
    >
      Original themes
    </div>
    {#each originals as theme}
      <label class="appearance-picker__choice mx-2">
        <HugeiconsIcon
          icon={theme.icon}
          size={16}
          strokeWidth={1.5}
          aria-hidden="true"
        />
        <span class="flex-1 text-sm">{theme.name}</span>
        <span class="text-rp-subtle"
          >{theme.mode === 'day' ? 'Light' : 'Dark'}</span
        >
        <input
          type="radio"
          name={popoverID}
          class="radio radio-xs"
          aria-label={`${theme.name} ${theme.mode === 'day' ? 'Light' : 'Dark'}`}
          checked={selected === theme.id}
          onchange={() => {
            applyOriginal(theme)
            close()
          }}
        />
      </label>
    {/each}
  </fieldset>
</div>

<style lang="postcss">
  @reference "../../app.css";

  .appearance-picker__row {
    @apply grid items-center gap-1 px-2 py-1;
    grid-template-columns: 1fr auto auto;
  }

  .appearance-picker__choice {
    @apply flex cursor-pointer items-center gap-1.5 rounded-md px-2 py-2 text-xs;
  }

  .appearance-picker__choice:hover,
  .appearance-picker__choice:has(input:checked) {
    background: var(--item-selected-bg);
  }

  .appearance-picker__icon,
  .appearance-picker__fade {
    display: inline-grid;
    place-items: center;
  }

  .appearance-picker__fade {
    grid-area: 1 / 1;
  }
</style>

<script lang="ts">
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { PaintBrush01Icon, Tick02Icon } from '@hugeicons/core-free-icons'
  import FilterPopover from '@/components/shared/Toolbar/FilterPopover.svelte'
  import { COUNT_PALETTES } from '@/themes/count-palettes'
  import { countSwatches } from '@/utils/chart-palette'
  import { countPalette } from '@/state/count-palette.svelte'
  import { themeSignal } from '@/state/theme.svelte'

  let { label = 'Count palette' }: { label?: string } = $props()

  const options = $derived(
    COUNT_PALETTES.map(palette => ({
      ...palette,
      swatches: countSwatches(12, themeSignal.value, palette.id),
    }))
  )
</script>

<FilterPopover
  {label}
  compact
  triggerClass="header-icon-button tooltip tooltip-left"
  focusSelected
>
  {#snippet icon()}
    <HugeiconsIcon
      icon={PaintBrush01Icon}
      size={16}
      strokeWidth={1.5}
      aria-hidden="true"
    />
  {/snippet}
  {#snippet children(close)}
    {#each options as option (option.id)}
      <button
        type="button"
        class="item-state item-state--menu flex w-full cursor-pointer items-center gap-3 px-3 py-2 text-left font-sans text-sm font-normal"
        aria-pressed={option.id === countPalette.value}
        onclick={() => {
          countPalette.set(option.id)
          close()
        }}
      >
        <span class="min-w-0 flex-1">{option.name}</span>
        <span
          class="flex h-4 w-28 shrink-0 overflow-hidden rounded-sm"
          aria-hidden="true"
        >
          {#each option.swatches as swatch}
            <span class="h-full flex-1" style:background-color={swatch}></span>
          {/each}
        </span>
        <span class="size-4 shrink-0" aria-hidden="true">
          {#if option.id === countPalette.value}
            <HugeiconsIcon icon={Tick02Icon} size={16} strokeWidth={1.5} />
          {/if}
        </span>
      </button>
    {/each}
  {/snippet}
  {#snippet footer()}
    <a
      class="link text-sm text-rp-subtle"
      href="https://madoromi.moonwit.ch/"
      target="_blank"
      rel="noreferrer"
    >Inks by Madoromi</a>
  {/snippet}
</FilterPopover>

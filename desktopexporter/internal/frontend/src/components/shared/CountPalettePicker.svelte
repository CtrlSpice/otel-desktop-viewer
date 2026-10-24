<script lang="ts">
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { HeartIcon, PaintBrush01Icon, Tick02Icon } from '@hugeicons/core-free-icons'
  import FilterPopover from '@/components/shared/Toolbar/FilterPopover.svelte'
  import FilterInput from '@/components/shared/Search/FilterInput.svelte'
  import { COUNT_PALETTES } from '@/themes/count-palettes'
  import { countSwatches } from '@/utils/chart-palette'
  import { countPalette } from '@/state/count-palette.svelte'
  import { themeSignal } from '@/state/theme.svelte'

  let { label = 'Count palette' }: { label?: string } = $props()
  let search = $state('')
  let optionsList = $state<HTMLDivElement>()
  const query = $derived(search.trim().toLowerCase().replaceAll('-', ' '))

  const options = $derived(
    COUNT_PALETTES.map(palette => ({
      ...palette,
      swatches: countSwatches(5, themeSignal.value, palette.id),
    }))
  )
  const visibleOptions = $derived(
    options.filter(option =>
      `${option.name} ${option.english}`
        .toLowerCase()
        .replaceAll('-', ' ')
        .includes(query)
    )
  )

  function handleOpenChange(open: boolean) {
    if (!open) {
      search = ''
      return
    }
    const selected = optionsList?.querySelector('[aria-pressed="true"]')
    if (!optionsList || !selected) return
    // Reveal the focused saved choice inside this list without moving the page.
    const item = selected.getBoundingClientRect()
    const list = optionsList.getBoundingClientRect()
    optionsList.scrollTop +=
      item.top - list.top - (optionsList.clientHeight - item.height) / 2
  }
</script>

<FilterPopover
  {label}
  compact
  triggerClass="header-icon-button tooltip tooltip-left"
  focusSelected
  showHeader={false}
  onOpenChange={handleOpenChange}
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
    <div class="mb-2 shrink-0">
      <FilterInput
        bind:value={search}
        label="Find ink"
        placeholder="Find an ink…"
      />
    </div>
    <div
      class="count-palette-options anchor-popover__body"
      bind:this={optionsList}
    >
      {#each visibleOptions as option (option.id)}
        <button
          type="button"
          class="item-state item-state--menu flex w-full cursor-pointer items-center gap-3 px-3 py-2 text-left font-sans text-sm font-normal"
          aria-pressed={option.id === countPalette.value}
          onclick={() => {
            countPalette.set(option.id)
            close()
          }}
        >
          <span class="min-w-0 flex-1">
            {option.name} <span class="text-rp-subtle">({option.english})</span>
          </span>
          <span class="flex shrink-0 items-center gap-2" aria-hidden="true">
            {#each option.swatches as swatch}
              <span
                class="size-2 shrink-0 rounded-full"
                style:background-color={swatch}
              ></span>
            {/each}
          </span>
          <span class="size-4 shrink-0" aria-hidden="true">
            {#if option.id === countPalette.value}
              <HugeiconsIcon icon={Tick02Icon} size={16} strokeWidth={1.5} />
            {/if}
          </span>
        </button>
      {:else}
        <p class="px-3 py-2 text-sm text-rp-subtle">No matching inks.</p>
      {/each}
    </div>
  {/snippet}
  {#snippet footer()}
    <a
      class="link mx-auto inline-flex items-center gap-2 text-sm text-rp-subtle"
      href="https://madoromi.moonwit.ch/"
      target="_blank"
      rel="noreferrer"
    >
      <span>Inks by Madoromi</span>
      <HugeiconsIcon
        icon={HeartIcon}
        size={16}
        strokeWidth={1.5}
        class="shrink-0 text-(--color-rose)"
        aria-hidden="true"
      />
    </a>
  {/snippet}
</FilterPopover>

<style>
  .count-palette-options {
    max-height: min(55vh, 22rem);
  }
</style>

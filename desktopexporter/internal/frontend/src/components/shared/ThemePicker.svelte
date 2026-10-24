<script lang="ts">
  import { tick } from 'svelte'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import {
    ArrowLeft01Icon,
    ArrowRight01Icon,
    FlowerIcon,
    HeartIcon,
    Leaf02Icon,
    MonitorIcon,
    SnowflakeIcon,
    Sun03Icon,
  } from '@hugeicons/core-free-icons'
  import FilterPopover from './Toolbar/FilterPopover.svelte'
  import ThemePreview from './ThemePreview.svelte'
  import { ACCENT_INKS } from '@/themes/accent-inks'
  import { SEASONS, type Season, type Theme } from '@/themes'
  import { accentInk } from '@/state/accent-ink.svelte'

  let {
    theme,
    onSeasonChange,
  }: {
    theme: Theme
    onSeasonChange: (season: Season) => void
  } = $props()

  const seasonIcons = {
    haru: FlowerIcon,
    natsu: Sun03Icon,
    aki: Leaf02Icon,
    fuyu: SnowflakeIcon,
  }
  const inks = ACCENT_INKS.map(ink => ({
    ...ink,
    preview: ink.swatches
      .filter((_, index) => index % 3 === 0)
      .map(swatch => swatch.color),
  }))
  const chosen = $derived(inks.find(ink => ink.id === accentInk.value)!)
  let optionsList = $state<HTMLDivElement>()
  let open = $state(false)
  let canScrollLeft = $state(false)
  let canScrollRight = $state(false)

  function updateInkScroll() {
    if (!optionsList) return
    canScrollLeft = optionsList.scrollLeft > 1
    canScrollRight =
      optionsList.scrollLeft + optionsList.clientWidth <
      optionsList.scrollWidth - 1
  }

  $effect(() => {
    if (!open || !optionsList) return
    const observer = new ResizeObserver(updateInkScroll)
    observer.observe(optionsList)
    updateInkScroll()
    return () => observer.disconnect()
  })

  function scrollInks(direction: -1 | 1) {
    if (!optionsList || (direction === -1 ? !canScrollLeft : !canScrollRight))
      return
    optionsList.scrollBy({
      left: direction * Math.round(optionsList.clientWidth * 0.75),
      behavior: 'instant',
    })
  }

  async function focusInk(nextOpen: boolean) {
    open = nextOpen
    if (!nextOpen) return
    await tick()
    if (!open || !optionsList) return
    const button = optionsList.querySelector<HTMLButtonElement>(
      '[aria-checked="true"]'
    )
    if (!button) return
    button.focus({ preventScroll: true })
    button.scrollIntoView({ block: 'nearest', inline: 'nearest' })
    updateInkScroll()
  }

  function selectInk(event: KeyboardEvent, index: number) {
    let next: number
    if (event.key === 'ArrowRight' || event.key === 'ArrowDown') {
      next = (index + 1) % inks.length
    } else if (event.key === 'ArrowLeft' || event.key === 'ArrowUp') {
      next = (index - 1 + inks.length) % inks.length
    } else if (event.key === 'Home') {
      next = 0
    } else if (event.key === 'End') {
      next = inks.length - 1
    } else {
      return
    }
    event.preventDefault()
    const ink = inks[next]!
    accentInk.set(ink.id)
    optionsList
      ?.querySelector<HTMLButtonElement>(`[data-ink-choice="${ink.id}"]`)
      ?.focus()
  }
</script>

{#snippet swatches(colors: readonly string[], vertical = false)}
  <span
    class="flex shrink-0 items-center gap-1"
    class:flex-col-reverse={vertical}
    aria-hidden="true"
  >
    {#each colors as color}
      <span class="size-2 shrink-0 rounded-full" style:background-color={color}
      ></span>
    {/each}
  </span>
{/snippet}

<FilterPopover
  label={`Theme: ${chosen.name}`}
  tooltip={chosen.name}
  anchor="below-end"
  role="dialog"
  compact
  triggerClass="btn btn-ghost btn-sm chart-header-control theme-ink tooltip tooltip-bottom md:tooltip-left"
  popoverClass="theme-picker-popover"
  showHeader={false}
  onOpenChange={focusInk}
>
  {#snippet icon()}
    {@render swatches(chosen.preview)}
    <HugeiconsIcon
      icon={MonitorIcon}
      size={18}
      strokeWidth={1.5}
      class="shrink-0"
      aria-hidden="true"
    />
  {/snippet}
  {#snippet children()}
    <div class="theme-picker__layout">
      <div class="theme-picker__controls">
        <fieldset class="theme-picker__seasons">
          <legend class="sr-only">Background</legend>
          <div class="grid grid-cols-4 gap-1">
            {#each SEASONS as season (season.id)}
              <button
                type="button"
                class="theme-picker__season item-state item-state--menu"
                aria-label={`${season.japanese} background`}
                title={season.japanese}
                aria-pressed={theme.season === season.id}
                onclick={() => onSeasonChange(season.id)}
              >
                <HugeiconsIcon
                  icon={seasonIcons[season.id]}
                  size={16}
                  strokeWidth={1.5}
                  aria-hidden="true"
                />
              </button>
            {/each}
          </div>
        </fieldset>
        <p class="theme-picker__current text-center text-sm">
          {chosen.name}
        </p>
        <fieldset class="m-0 min-w-0 border-0 p-0">
          <legend class="sr-only">Ink</legend>
          <div class="theme-picker__ink-strip">
            <button
              type="button"
              class="btn btn-ghost btn-circle btn-sm"
              class:btn-disabled={!canScrollLeft}
              aria-label="Scroll inks left"
              title="Scroll inks left"
              aria-disabled={!canScrollLeft}
              onclick={() => scrollInks(-1)}
            >
              <HugeiconsIcon
                icon={ArrowLeft01Icon}
                size={18}
                strokeWidth={1.5}
                aria-hidden="true"
              />
            </button>
            <div
              class="theme-picker__inks"
              role="radiogroup"
              aria-label="Theme inks"
              bind:this={optionsList}
              onscroll={updateInkScroll}
            >
              {#each inks as ink, index (ink.id)}
                <button
                  type="button"
                  role="radio"
                  class="theme-picker__choice item-state"
                  data-ink-choice={ink.id}
                  aria-label={ink.name}
                  title={ink.name}
                  aria-checked={ink.id === accentInk.value}
                  tabindex={ink.id === accentInk.value ? 0 : -1}
                  onkeydown={event => selectInk(event, index)}
                  onclick={() => accentInk.set(ink.id)}
                >
                  {@render swatches(ink.preview, true)}
                </button>
              {/each}
            </div>
            <button
              type="button"
              class="btn btn-ghost btn-circle btn-sm"
              class:btn-disabled={!canScrollRight}
              aria-label="Scroll inks right"
              title="Scroll inks right"
              aria-disabled={!canScrollRight}
              onclick={() => scrollInks(1)}
            >
              <HugeiconsIcon
                icon={ArrowRight01Icon}
                size={18}
                strokeWidth={1.5}
                aria-hidden="true"
              />
            </button>
          </div>
        </fieldset>
      </div>
      {#if open}<ThemePreview />{/if}
    </div>
  {/snippet}
  {#snippet footer()}
    <div
      class="flex w-full items-center justify-center gap-1 text-sm text-subtle"
    >
      <span>Colours:</span>
      <a
        class="link inline-flex items-center gap-2"
        href="https://madoromi.moonwit.ch/"
        target="_blank"
        rel="noreferrer"
      >
        Madoromi
        <HugeiconsIcon
          icon={HeartIcon}
          size={16}
          strokeWidth={1.5}
          class="text-(--color-sakura)"
          aria-hidden="true"
        />
      </a>
    </div>
  {/snippet}
</FilterPopover>

<style lang="postcss">
  @reference '../../app.css';

  :global(.filter-popover.theme-picker-popover) {
    width: 34rem;
    padding: 1.25rem;
    max-height: calc(100dvh - 2rem);
  }

  .theme-picker__layout {
    @apply flex min-w-0 flex-col gap-4;
  }

  .theme-picker__controls {
    @apply flex min-w-0 flex-col gap-3;
  }

  .theme-picker__choice {
    @apply flex h-12 w-8 shrink-0 cursor-pointer items-center justify-center rounded-full p-0;
  }

  .theme-picker__season {
    @apply flex items-center justify-center p-2;
  }

  .theme-picker__seasons {
    @apply m-0 mx-auto w-full min-w-0 max-w-64 shrink-0 border-0 p-0;
  }

  .theme-picker__inks {
    @apply mx-auto grid max-w-full grid-flow-col gap-0.5 overflow-x-auto py-1;
    width: max-content;
    grid-auto-columns: 2rem;
  }

  .theme-picker__ink-strip {
    @apply grid min-w-0 items-center gap-1;
    grid-template-columns: auto minmax(0, 1fr) auto;
  }
</style>

<script lang="ts">
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import {
    FlowerIcon,
    HeartIcon,
    Leaf02Icon,
    MonitorIcon,
    SnowflakeIcon,
    Sun03Icon,
  } from '@hugeicons/core-free-icons'
  import FilterPopover from './Toolbar/FilterPopover.svelte'
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

  function focusInk(open: boolean) {
    if (!open || !optionsList) return
    const button = optionsList.querySelector<HTMLButtonElement>(
      '[aria-pressed="true"]'
    )
    if (!button) return
    button.focus({ preventScroll: true })
    const item = button.getBoundingClientRect()
    const list = optionsList.getBoundingClientRect()
    optionsList.scrollTop +=
      item.top - list.top - (optionsList.clientHeight - item.height) / 2
  }
</script>

{#snippet swatches(colors: readonly string[])}
  <span class="flex shrink-0 items-center gap-1" aria-hidden="true">
    {#each colors as color}
      <span class="size-2 shrink-0 rounded-full" style:background-color={color}
      ></span>
    {/each}
  </span>
{/snippet}

<FilterPopover
  label={`Theme: ${chosen.name}\n(${chosen.english})`}
  compact
  triggerClass="btn btn-ghost btn-sm chart-header-control theme-ink tooltip tooltip-bottom md:tooltip-left before:whitespace-pre-line"
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
  {#snippet children(close)}
    <fieldset class="m-0 mb-2 min-w-0 shrink-0 border-0 p-0">
      <legend class="sr-only">Background</legend>
      <div class="grid grid-cols-4 gap-1">
        {#each SEASONS as season (season.id)}
          <button
            type="button"
            class="theme-picker__season item-state item-state--menu"
            aria-label={`${season.japanese} (${season.name}) background`}
            title={season.name}
            aria-pressed={theme.season === season.id}
            onclick={() => {
              close()
              onSeasonChange(season.id)
            }}
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
    <fieldset class="m-0 min-h-0 min-w-0 border-0 p-0">
      <legend class="sr-only">Ink</legend>
      <div
        class="theme-picker__inks theme-picker__choices anchor-popover__body"
        bind:this={optionsList}
      >
        {#each inks as ink (ink.id)}
          <button
            type="button"
            class="theme-picker__choice item-state item-state--menu"
            data-ink-choice={ink.id}
            aria-label={`${ink.name} (${ink.english})`}
            title={ink.english}
            aria-pressed={ink.id === accentInk.value}
            onclick={() => {
              accentInk.set(ink.id)
              close()
            }}
          >
            {@render swatches(ink.preview)}
            <span>{ink.name}</span>
          </button>
        {/each}
      </div>
    </fieldset>
  {/snippet}
  {#snippet footer()}
    <a
      class="link mx-auto inline-flex items-center gap-2 text-sm text-subtle"
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
  {/snippet}
</FilterPopover>

<style lang="postcss">
  @reference '../../app.css';

  :global(.filter-popover.theme-picker-popover) {
    width: 22rem;
  }

  .theme-picker__choice {
    @apply flex min-w-0 items-center gap-2 px-2 py-1.5 text-left text-sm font-normal;
  }

  .theme-picker__season {
    @apply flex items-center justify-center p-2;
  }

  .theme-picker__choices {
    @apply grid grid-cols-2 gap-1;
  }

  .theme-picker__inks {
    max-height: min(45vh, 20rem);
  }

  @media (max-width: 22.5rem) {
    .theme-picker__choices {
      grid-template-columns: minmax(0, 1fr);
    }
  }
</style>

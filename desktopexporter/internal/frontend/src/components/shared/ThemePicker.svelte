<script lang="ts">
  import { tick } from 'svelte'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import {
    FlowerIcon,
    HeartIcon,
    Leaf02Icon,
    SnowflakeIcon,
    Sun03Icon,
  } from '@hugeicons/core-free-icons'
  import FilterPopover from './Toolbar/FilterPopover.svelte'
  import { SEASONS, type Season, type Theme } from '@/themes'

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
  const chosen = $derived(SEASONS.find(season => season.id === theme.season)!)
  let optionsList = $state<HTMLDivElement>()
  let open = $state(false)

  async function focusSeason(nextOpen: boolean) {
    open = nextOpen
    if (!open) return
    await tick()
    if (open)
      optionsList
        ?.querySelector<HTMLButtonElement>('[aria-checked="true"]')
        ?.focus({ preventScroll: true })
  }

  function selectSeason(event: KeyboardEvent, index: number) {
    let next: number
    if (event.key === 'ArrowRight' || event.key === 'ArrowDown')
      next = (index + 1) % SEASONS.length
    else if (event.key === 'ArrowLeft' || event.key === 'ArrowUp')
      next = (index - 1 + SEASONS.length) % SEASONS.length
    else if (event.key === 'Home') next = 0
    else if (event.key === 'End') next = SEASONS.length - 1
    else return
    event.preventDefault()
    const season = SEASONS[next]!
    onSeasonChange(season.id)
    optionsList
      ?.querySelector<HTMLButtonElement>(`[data-season="${season.id}"]`)
      ?.focus()
  }
</script>

<FilterPopover
  label={`Season: ${chosen.japanese}`}
  tooltip={chosen.japanese}
  anchor="below-end"
  role="dialog"
  compact
  triggerClass="btn btn-ghost btn-sm chart-header-control theme-season tooltip tooltip-bottom md:tooltip-left"
  popoverClass="theme-picker-popover"
  showHeader={false}
  onOpenChange={focusSeason}
>
  {#snippet icon()}
    <HugeiconsIcon
      icon={seasonIcons[theme.season]}
      size={18}
      strokeWidth={1.5}
      aria-hidden="true"
    />
  {/snippet}
  {#snippet children()}
    <div
      class="theme-picker__seasons"
      role="radiogroup"
      aria-label="Season"
      bind:this={optionsList}
    >
      {#each SEASONS as season, index (season.id)}
        <button
          type="button"
          role="radio"
          class="theme-picker__season item-state item-state--menu"
          data-season={season.id}
          aria-label={season.japanese}
          aria-checked={theme.season === season.id}
          tabindex={theme.season === season.id ? 0 : -1}
          onkeydown={event => selectSeason(event, index)}
          onclick={() => onSeasonChange(season.id)}
        >
          <HugeiconsIcon
            icon={seasonIcons[season.id]}
            size={18}
            strokeWidth={1.5}
            aria-hidden="true"
          />
          <span>{season.japanese}</span>
        </button>
      {/each}
    </div>
  {/snippet}
  {#snippet footer()}
    <div
      class="flex w-full items-center justify-center gap-1 text-sm text-subtle"
    >
      <span>Colours by</span>
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
    width: 18rem;
    padding: 0.75rem;
  }
  .theme-picker__seasons {
    @apply grid grid-cols-4 gap-1;
  }
  .theme-picker__season {
    @apply flex flex-col items-center justify-center gap-2 p-2;
  }
</style>

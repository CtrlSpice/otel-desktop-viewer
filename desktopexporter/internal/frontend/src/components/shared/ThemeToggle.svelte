<script lang="ts">
  import { onMount } from 'svelte'
  import { fade } from 'svelte/transition'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import {
    ArrowDown01Icon,
    FlowerIcon,
    Leaf02Icon,
    Moon02Icon,
    SnowflakeIcon,
    Sun03Icon,
  } from '@hugeicons/core-free-icons'
  import { createPopoverID, setupAnchorPopover } from './utils/anchor-popover'
  import {
    DEFAULT_THEME,
    SEASONS,
    getTheme,
    restoreTheme,
    themeName,
    type Season,
    type ThemeName,
  } from '@/themes'

  let { class: className = '' }: { class?: string } = $props()

  let currentTheme = $state<ThemeName>(DEFAULT_THEME)
  const seasonIcons = {
    haru: FlowerIcon,
    natsu: Sun03Icon,
    aki: Leaf02Icon,
    fuyu: SnowflakeIcon,
  }
  const popoverID = createPopoverID('theme-season')
  let trigger = $state<HTMLButtonElement>()
  let popover = $state<HTMLDivElement>()
  let open = $state(false)

  $effect(() => {
    if (!trigger || !popover) return
    const menu = popover
    return setupAnchorPopover({
      trigger,
      popover: menu,
      anchor: 'below-end',
      onOpenChange: value => {
        open = value
        if (value)
          menu
            .querySelector<HTMLButtonElement>('[aria-pressed="true"]')
            ?.focus()
      },
    })
  })

  function setTheme(theme: ThemeName) {
    currentTheme = theme
    document.documentElement.setAttribute('data-theme', theme)
    document.documentElement.setAttribute(
      'data-color-scheme',
      getTheme(theme).scheme
    )
    document.documentElement.removeAttribute('data-palette')
    try {
      localStorage.setItem('theme', theme)
      localStorage.removeItem('theme-palette')
    } catch {
      // Theme controls still work when browser storage is unavailable.
    }
  }

  function changeTheme(next: ThemeName) {
    if (document.startViewTransition) {
      document.startViewTransition(() => setTheme(next))
    } else {
      setTheme(next)
    }
  }

  function toggleMode() {
    changeTheme(
      themeName(theme.season, theme.scheme === 'light' ? 'dark' : 'light')
    )
  }

  function changeSeason(season: Season) {
    popover?.hidePopover()
    trigger?.focus()
    changeTheme(themeName(season, theme.scheme))
  }

  onMount(() => {
    let saved: string | null = null
    try {
      saved = localStorage.getItem('theme')
    } catch {
      // Fall back to the system mode when browser storage is unavailable.
    }
    setTheme(
      restoreTheme(
        saved,
        window.matchMedia('(prefers-color-scheme: dark)').matches
      ).id
    )
  })

  const theme = $derived(getTheme(currentTheme))
  // Announce the current state and show the mode-changing action on hover.
  const label = $derived(`${theme.name} theme active`)
</script>

<!--
  data-tip rather than title: callers add a `tooltip tooltip-*` class and get
  the DaisyUI bubble in the same style as the rest of the drawer chrome. Both
  at once would stack a native tooltip under the styled one.
-->
<div class="theme-controls" role="group" aria-label="Theme">
  <button
    bind:this={trigger}
    type="button"
    class="btn btn-ghost btn-sm chart-header-control theme-season"
    aria-label="Season: {SEASONS.find(season => season.id === theme.season)
      ?.name}"
    aria-expanded={open}
    aria-controls={popoverID}
    popovertarget={popoverID}
  >
    {#key theme.season}
      <HugeiconsIcon
        icon={seasonIcons[theme.season]}
        size={16}
        strokeWidth={1.5}
        aria-hidden="true"
      />
    {/key}
    <span>{SEASONS.find(season => season.id === theme.season)?.name}</span>
    <HugeiconsIcon
      icon={ArrowDown01Icon}
      size={12}
      strokeWidth={1.5}
      aria-hidden="true"
    />
  </button>
  <button
    class="theme-toggle {className}"
    data-tip="Switch to {theme.scheme === 'light' ? 'night' : 'day'}"
    aria-label={label}
    aria-pressed={theme.scheme === 'dark'}
    onclick={toggleMode}
  >
    <span class="theme-toggle__icon">
      {#key theme.scheme}
        <span class="theme-toggle__fade" transition:fade={{ duration: 300 }}>
          {#if theme.scheme === 'light'}
            <HugeiconsIcon
              icon={Sun03Icon}
              size="1em"
              strokeWidth={1.5}
              class="h-[17px] w-[17px] shrink-0"
            />
          {:else}
            <HugeiconsIcon
              icon={Moon02Icon}
              size="1em"
              strokeWidth={1.5}
              class="h-[17px] w-[17px] shrink-0"
            />
          {/if}
        </span>
      {/key}
    </span>
  </button>
</div>
<div
  bind:this={popover}
  id={popoverID}
  popover="auto"
  class="anchor-popover anchor-popover--anchored season-popover"
  role="group"
  aria-label="Seasons"
>
  {#each SEASONS as season (season.id)}
    <button
      type="button"
      aria-pressed={theme.season === season.id}
      onclick={() => changeSeason(season.id)}
      class="item-state item-state--menu flex w-full items-center gap-2 px-3 py-2 text-left text-sm font-normal"
    >
      <HugeiconsIcon
        icon={seasonIcons[season.id]}
        size={16}
        strokeWidth={1.5}
        aria-hidden="true"
      />
      {season.name}
    </button>
  {/each}
</div>

<style lang="postcss">
  @reference "../../app.css";

  .theme-controls {
    @apply inline-flex shrink-0 items-center rounded-lg border border-base-300 bg-base-200;
  }

  .theme-season {
    @apply gap-2 rounded-r-none;
  }

  .theme-toggle {
    @apply rounded-l-none rounded-r-lg;
    border-left: 1px solid var(--color-base-300);
  }

  .season-popover {
    @apply gap-1 p-2;
    width: 11rem;
    max-width: calc(100vw - 1rem);
  }

  .theme-toggle__icon {
    display: inline-grid;
    place-items: center;
  }

  .theme-toggle__fade {
    grid-area: 1 / 1;
    display: inline-grid;
    place-items: center;
  }
</style>

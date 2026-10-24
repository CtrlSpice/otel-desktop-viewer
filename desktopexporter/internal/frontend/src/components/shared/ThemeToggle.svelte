<script lang="ts">
  import { onMount } from 'svelte'
  import { fade } from 'svelte/transition'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { Moon02Icon, Sun03Icon } from '@hugeicons/core-free-icons'
  import ThemePicker from './ThemePicker.svelte'
  import {
    DEFAULT_THEME,
    getTheme,
    restoreTheme,
    themeName,
    type Season,
    type ThemeName,
  } from '@/themes'

  let { class: className = '' }: { class?: string } = $props()

  let currentTheme = $state<ThemeName>(DEFAULT_THEME)

  function setTheme(theme: ThemeName) {
    currentTheme = theme
    document.documentElement.setAttribute('data-theme', theme)
    document.documentElement.setAttribute(
      'data-color-scheme',
      getTheme(theme).scheme
    )
    try {
      localStorage.setItem('theme', theme)
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
<div class="theme-controls" role="group" aria-label="Appearance">
  <ThemePicker {theme} onSeasonChange={changeSeason} />
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

<style lang="postcss">
  @reference "../../app.css";

  .theme-controls {
    @apply inline-flex shrink-0 items-center rounded-full border-0 bg-base-200;
  }

  .theme-controls :global(.theme-ink) {
    @apply gap-2 rounded-l-full rounded-r-none;
  }

  .theme-toggle {
    @apply rounded-l-none rounded-r-full font-normal;
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

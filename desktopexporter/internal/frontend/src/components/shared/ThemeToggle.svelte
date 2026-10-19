<script lang="ts">
  import { onMount } from 'svelte'
  import { fade } from 'svelte/transition'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import {
    FallingStarIcon,
    Moon02Icon,
    Sun03Icon,
  } from '@hugeicons/core-free-icons'
  import {
    DEFAULT_THEME,
    THEMES,
    getTheme,
    restoreTheme,
    type ThemeName,
  } from '@/themes'

  const CYCLE = THEMES.map(theme => theme.id)

  let { class: className = '' }: { class?: string } = $props()

  let currentTheme = $state<ThemeName>(DEFAULT_THEME)

  function setTheme(theme: ThemeName) {
    currentTheme = theme
    localStorage.setItem('theme', theme)
    document.documentElement.setAttribute('data-theme', theme)
    document.documentElement.removeAttribute('data-palette')
    localStorage.removeItem('theme-palette')
  }

  function cycleTheme() {
    const idx = CYCLE.indexOf(currentTheme)
    const next = CYCLE[(idx + 1) % CYCLE.length]
    if (document.startViewTransition) {
      document.startViewTransition(() => setTheme(next))
    } else {
      setTheme(next)
    }
  }

  onMount(() => {
    setTheme(
      restoreTheme(
        localStorage.getItem('theme'),
        window.matchMedia('(prefers-color-scheme: dark)').matches
      ).id
    )
  })

  const themeName = $derived(getTheme(currentTheme).name)
  // aria-label states current state ("active"); the tooltip just names the
  // theme, since a hovering user already sees which one is showing.
  const label = $derived(`${themeName} theme active`)
</script>

<!--
  data-tip rather than title: callers add a `tooltip tooltip-*` class and get
  the DaisyUI bubble in the same style as the rest of the drawer chrome. Both
  at once would stack a native tooltip under the styled one.
-->
<button
  class="theme-toggle {className}"
  data-tip="{themeName} theme"
  aria-label={label}
  onclick={cycleTheme}
>
  <span class="theme-toggle__icon">
    {#key currentTheme}
      <span class="theme-toggle__fade" transition:fade={{ duration: 300 }}>
        {#if currentTheme === 'rose-pine-dawn'}
          <HugeiconsIcon
            icon={Sun03Icon}
            size="1em"
            strokeWidth={1.5}
            class="h-[17px] w-[17px] shrink-0"
          />
        {:else if currentTheme === 'rose-pine-moon'}
          <HugeiconsIcon
            icon={Moon02Icon}
            size="1em"
            strokeWidth={1.5}
            class="h-[17px] w-[17px] shrink-0"
          />
        {:else}
          <HugeiconsIcon
            icon={FallingStarIcon}
            size="1em"
            strokeWidth={1.5}
            class="h-[17px] w-[17px] shrink-0"
          />
        {/if}
      </span>
    {/key}
  </span>
</button>

<style lang="postcss">
  @reference "../../app.css";

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

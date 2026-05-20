<script lang="ts">
  import { onMount } from 'svelte'
  import { fade } from 'svelte/transition'
  import { SunIcon, MoonIcon, FallingStarIcon } from '@/icons'

  type ThemeName = 'rose-pine-dawn' | 'rose-pine-moon' | 'rose-pine'

  const CYCLE: ThemeName[] = ['rose-pine-dawn', 'rose-pine-moon', 'rose-pine']
  const VALID = new Set<string>(CYCLE)

  let { class: className = '' }: { class?: string } = $props()

  let currentTheme = $state<ThemeName>('rose-pine-moon')

  function setTheme(theme: ThemeName) {
    currentTheme = theme
    localStorage.setItem('theme', theme)
    document.documentElement.setAttribute('data-theme', theme)
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
    const saved = localStorage.getItem('theme')
    if (saved && VALID.has(saved)) {
      setTheme(saved as ThemeName)
    } else {
      const prefersDark = window.matchMedia(
        '(prefers-color-scheme: dark)',
      ).matches
      setTheme(prefersDark ? 'rose-pine-moon' : 'rose-pine-dawn')
    }
  })

  const label = $derived(
    currentTheme === 'rose-pine-dawn'
      ? 'Dawn theme active'
      : currentTheme === 'rose-pine-moon'
        ? 'Moon theme active'
        : 'Pine theme active',
  )
</script>

<button
  class="theme-toggle {className}"
  title={label}
  aria-label={label}
  onclick={cycleTheme}
>
  <span class="theme-toggle__icon">
    {#key currentTheme}
      <span class="theme-toggle__fade" transition:fade={{ duration: 300 }}>
        {#if currentTheme === 'rose-pine-dawn'}
          <SunIcon class="h-[17px] w-[17px] shrink-0" />
        {:else if currentTheme === 'rose-pine-moon'}
          <MoonIcon class="h-[17px] w-[17px] shrink-0" />
        {:else}
          <FallingStarIcon class="h-[17px] w-[17px] shrink-0" />
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

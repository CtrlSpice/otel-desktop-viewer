<script lang="ts">
  import { onMount } from 'svelte'

  type ThemeName = 'rose-pine-dawn' | 'rose-pine-moon'

  let { class: className = '' }: { class?: string } = $props()

  let currentTheme = $state<ThemeName>('rose-pine-moon')

  function setTheme(theme: ThemeName) {
    currentTheme = theme
    localStorage.setItem('theme', currentTheme)
    document.documentElement.setAttribute('data-theme', currentTheme)
  }

  function syncToSystem() {
    window.matchMedia('(prefers-color-scheme: dark)').matches
      ? setTheme('rose-pine-moon')
      : setTheme('rose-pine-dawn')
  }

  function toggleTheme() {
    setTheme(currentTheme === 'rose-pine-dawn' ? 'rose-pine-moon' : 'rose-pine-dawn')
  }

  onMount(() => {
    let savedTheme = localStorage.getItem('theme')
    if (
      savedTheme &&
      ['rose-pine-dawn', 'rose-pine-moon'].includes(savedTheme)
    ) {
      setTheme(savedTheme as ThemeName)
    } else {
      syncToSystem()
    }

    let mediaQuery = window.matchMedia('(prefers-color-scheme: dark)')
    mediaQuery.addEventListener('change', syncToSystem)

    document.documentElement.setAttribute('data-theme', currentTheme)

    return () => {
      mediaQuery.removeEventListener('change', syncToSystem)
    }
  })

  let isDark = $derived(currentTheme === 'rose-pine-moon')
</script>

<!-- DaisyUI swap + swap-rotate; icons stay our stroke SVGs -->
<label
  class="theme-toggle-swap swap swap-rotate {className}"
  title="Toggle theme"
  aria-label="Toggle theme"
>
  <!-- this hidden checkbox controls the state -->
  <input type="checkbox" checked={isDark} onchange={toggleTheme} />

  <!-- sun (swap-on = checked / dark theme → click for light) -->
  <svg
    class="swap-on h-[17px] w-[17px] shrink-0 fill-current"
    xmlns="http://www.w3.org/2000/svg"
    viewBox="0 0 24 24"
    aria-hidden="true"
  >
    <g fill="none" stroke="currentColor" stroke-width="1.5">
      <path d="M17 12a5 5 0 1 1-10 0a5 5 0 0 1 10 0Z" />
      <path
        stroke-linecap="round"
        d="M12 2v1.5m0 17V22m7.07-2.929l-1.06-1.06M5.99 5.989L4.928 4.93M22 12h-1.5m-17 0H2m17.071-7.071l-1.06 1.06M5.99 18.011l-1.06 1.06"
      />
    </g>
  </svg>

  <!-- moon (swap-off = unchecked / light theme → click for dark) -->
  <svg
    class="swap-off h-[17px] w-[17px] shrink-0 fill-current"
    xmlns="http://www.w3.org/2000/svg"
    viewBox="0 0 24 24"
    aria-hidden="true"
  >
    <path
      fill="none"
      stroke="currentColor"
      stroke-linecap="round"
      stroke-linejoin="round"
      stroke-width="1.5"
      d="M21.5 14.078A8.557 8.557 0 0 1 9.922 2.5C5.668 3.497 2.5 7.315 2.5 11.873a9.627 9.627 0 0 0 9.627 9.627c4.558 0 8.376-3.168 9.373-7.422"
    />
  </svg>
</label>

<style lang="postcss">
  @reference "../app.css";

  /*
   * Swap stacks icons in one grid cell. Don't wrap the label in flex layouts that add gap
   * (e.g. some `.btn` rows); we force grid here so swap-on/off stack for rotate.
   */
  label.theme-toggle-swap {
    display: inline-grid !important;
    gap: 0 !important;
    place-items: center;
  }
</style>

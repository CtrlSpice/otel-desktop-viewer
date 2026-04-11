<script lang="ts">
  import { onMount } from "svelte"
  import { MoonIcon, SunIcon } from "@/icons"

  type ThemeName = "rose-pine-dawn" | "rose-pine-moon"
  let currentTheme: ThemeName = "rose-pine-moon"

  function setTheme(theme: ThemeName) {
    currentTheme = theme
    localStorage.setItem("theme", currentTheme)
    document.documentElement.setAttribute("data-theme", currentTheme)
  }

  function syncToSystem() {
    window.matchMedia("(prefers-color-scheme: dark)").matches
      ? setTheme("rose-pine-moon")
      : setTheme("rose-pine-dawn")
  }

  onMount(() => {
    // User has made a choice before, use their preference
    let savedTheme = localStorage.getItem("theme")
    if (savedTheme && ["rose-pine-dawn", "rose-pine-moon"].includes(savedTheme)) {
      setTheme(savedTheme as ThemeName)
    } else {
      // First time user, respect system preference
      syncToSystem()
    }

    // Listen for system theme changes
    let mediaQuery = window.matchMedia("(prefers-color-scheme: dark)")
    mediaQuery.addEventListener("change", syncToSystem)

    document.documentElement.setAttribute("data-theme", currentTheme)

    return () => {
      mediaQuery.removeEventListener("change", syncToSystem)
    }
  })
</script>

<!-- Theme Toggle -->
<div class="flex items-center gap-2 text-base-content/45">
  <SunIcon class="h-4 w-4 shrink-0" aria-hidden="true" />
  <label title="Toggle theme" class="flex items-center" for="theme-toggle">
    <input
      id="theme-toggle"
      name="theme-toggle"
      type="checkbox"
      class="toggle toggle-neutral toggle-sm"
      checked={currentTheme === "rose-pine-moon"}
      onchange={() => setTheme(currentTheme === "rose-pine-dawn" ? "rose-pine-moon" : "rose-pine-dawn")}
    />
  </label>
  <MoonIcon class="h-4 w-4 shrink-0" aria-hidden="true" />
</div>

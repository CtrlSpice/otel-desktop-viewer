<script lang="ts">
  import { onMount } from "svelte"
  import { HugeiconsIcon } from "@hugeicons/svelte"
  import { Sun03Icon, Moon02Icon } from "@hugeicons/core-free-icons"

  type ThemeMode = "light" | "dark"
  let currentTheme: ThemeMode = "dark"

  function setTheme(theme: ThemeMode) {
    currentTheme = theme
    localStorage.setItem("theme", currentTheme)
    document.documentElement.setAttribute("data-theme", currentTheme)
  }

  function syncToSystem() {
    window.matchMedia("(prefers-color-scheme: dark)").matches
      ? setTheme("dark")
      : setTheme("light")
  }

  onMount(() => {
    // User has made a choice before, use their preference
    if (localStorage.getItem("theme") !== null) {
      let savedTheme = localStorage.getItem("theme")
      if (savedTheme && ["light", "dark"].includes(savedTheme)) {
        setTheme(savedTheme as ThemeMode)
      }
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
<div class="fixed top-4 right-4 z-50 flex items-center gap-2 p-2">
  <HugeiconsIcon icon={Sun03Icon} size={16} />
  <label title="Toggle theme" class="flex items-center">
    <input
      type="checkbox"
      class="toggle"
      checked={currentTheme === "dark"}
      on:change={() => setTheme(currentTheme === "light" ? "dark" : "light")}
    />
  </label>
  <HugeiconsIcon icon={Moon02Icon} size={16} />
</div>

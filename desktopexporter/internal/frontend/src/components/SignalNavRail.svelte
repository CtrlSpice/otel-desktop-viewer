<script lang="ts">
  import type { Snippet } from 'svelte'
  import DrawerNavTabs from '@/components/DrawerNavTabs.svelte'
  import ThemeToggle from '@/components/ThemeToggle.svelte'

  type Props = { children: Snippet }
  let { children }: Props = $props()
</script>

<div class="nav-rail-shell">
  <aside class="nav-rail" aria-label="Primary navigation">
    <div class="nav-rail__nav">
      <DrawerNavTabs collapsed />
    </div>
    <div class="nav-rail__footer">
      <ThemeToggle />
    </div>
  </aside>

  <div class="nav-rail__content">
    {@render children()}
  </div>
</div>

<style lang="postcss">
  @reference "../app.css";

  .nav-rail-shell {
    @apply flex min-h-0 min-w-0 w-full flex-1;
  }

  .nav-rail {
    @apply flex h-full w-14 shrink-0 flex-col items-center bg-base-100/95 px-1.5 py-2;
    border-right: 1px solid
      color-mix(in oklab, var(--color-base-300) 70%, transparent);
  }

  .nav-rail__nav {
    @apply flex-1 w-full;
  }

  .nav-rail__footer {
    @apply mt-auto flex items-center justify-center pt-2;
  }

  .nav-rail__content {
    @apply min-h-0 min-w-0 flex-1 overflow-auto;
  }
</style>

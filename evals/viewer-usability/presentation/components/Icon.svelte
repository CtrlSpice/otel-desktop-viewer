<script lang="ts">
  // The package's Svelte wrapper draws onMount. Static exports render its same
  // trusted glyph data directly, so icons also work without hydration.
  import type { Alert02Icon } from '@hugeicons/core-free-icons'
  let {
    icon,
    size = 20,
    strokeWidth = 1.5,
  }: {
    icon: typeof Alert02Icon
    size?: number
    strokeWidth?: number
  } = $props()

  const attributes = (values: Record<string, string | number>) =>
    Object.fromEntries(
      Object.entries(values)
        .filter(([name]) => name !== 'key')
        .map(([name, value]) => [
          name.replace(/[A-Z]/g, letter => '-' + letter.toLowerCase()),
          value,
        ])
    )
</script>

<svg
  width={size}
  height={size}
  viewBox="0 0 24 24"
  fill="none"
  aria-hidden="true"
  focusable="false"
>
  {#each icon as [tag, values]}
    <svelte:element this={tag} {...attributes({ ...values, strokeWidth })} />
  {/each}
</svg>

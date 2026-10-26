<script lang="ts">
  import type { Snippet } from 'svelte'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { Tick02Icon } from '@hugeicons/core-free-icons'

  let {
    selected,
    disabled = false,
    label,
    title,
    class: className = '',
    onclick,
    children,
  }: {
    selected: boolean
    disabled?: boolean
    label?: string
    title?: string
    class?: string
    onclick: () => void
    children: Snippet
  } = $props()
</script>

<button
  type="button"
  class="filter-option item-state {className}"
  aria-label={label}
  aria-pressed={selected}
  {disabled}
  {title}
  {onclick}
>
  <span class="flex min-w-0 flex-1 items-center gap-2"
    >{@render children()}</span
  >
  <span
    class="flex size-4 shrink-0 items-center justify-center"
    class:invisible={!selected}
    aria-hidden="true"
  >
    <HugeiconsIcon icon={Tick02Icon} size={16} strokeWidth={1.5} />
  </span>
</button>

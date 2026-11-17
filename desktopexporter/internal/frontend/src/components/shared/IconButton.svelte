<script lang="ts">
  import type { Snippet } from 'svelte'
  import type { HTMLButtonAttributes } from 'svelte/elements'

  let {
    label,
    tooltip = label,
    element = $bindable(null),
    class: className = '',
    children,
    ...attributes
  }: Omit<
    HTMLButtonAttributes,
    'children' | 'aria-label' | 'type' | 'class'
  > & {
    label: string
    tooltip?: string
    element?: HTMLButtonElement | null
    class?: string
    children: Snippet
  } = $props()
</script>

<button
  {...attributes}
  bind:this={element}
  type="button"
  class="item-state btn btn-ghost btn-sm text-sm font-normal text-base-content tooltip tooltip-bottom header-icon-button btn-circle {className}"
  aria-label={label}
  data-tip={tooltip}
>
  {@render children()}
</button>

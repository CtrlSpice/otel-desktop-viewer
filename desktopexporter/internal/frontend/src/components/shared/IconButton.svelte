<script lang="ts">
  import type { Snippet } from 'svelte'
  import type { HTMLButtonAttributes } from 'svelte/elements'

  let {
    label,
    tooltip = label,
    element = $bindable(null),
    class: className = '',
    children,
    text,
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
    text?: Snippet
  } = $props()
</script>

<button
  {...attributes}
  bind:this={element}
  type="button"
  class="item-state btn btn-ghost btn-sm text-sm font-normal text-base-content tooltip tooltip-bottom {text
    ? 'header-button min-w-0 gap-2 px-3'
    : 'header-icon-button btn-circle'} {className}"
  aria-label={label}
  data-tip={tooltip}
>
  {@render children()}
  {@render text?.()}
</button>

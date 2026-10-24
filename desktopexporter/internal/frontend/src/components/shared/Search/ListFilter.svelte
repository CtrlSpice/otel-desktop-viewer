<script lang="ts">
  import type { Snippet } from 'svelte'
  import type { HTMLInputAttributes } from 'svelte/elements'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { FilterIcon } from '@hugeicons/core-free-icons'
  import FilterInput from './FilterInput.svelte'

  let {
    label,
    placeholder = 'Filter',
    value = $bindable(''),
    onkeydown,
    children,
  }: {
    label: string
    placeholder?: string
    value?: string
    onkeydown?: HTMLInputAttributes['onkeydown']
    children: Snippet<[toggle: Snippet, field: Snippet]>
  } = $props()

  const id = $props.id()
  let open = $state(false)
  let trigger = $state<HTMLButtonElement>()
  let input = $state<HTMLInputElement>()
  const active = $derived(value.trim().length > 0)
  const accessibleLabel = $derived(`${label}${active ? ' (active)' : ''}`)

  $effect(() => {
    if (open) input?.focus({ preventScroll: true })
  })
</script>

{#snippet toggle()}
  <button
    bind:this={trigger}
    type="button"
    class="btn btn-ghost btn-circle btn-sm tooltip tooltip-left"
    class:btn-active={open}
    class:text-primary={active}
    aria-label={accessibleLabel}
    aria-expanded={open}
    aria-controls={id}
    data-tip={accessibleLabel}
    data-list-filter-toggle
    onclick={event => {
      event.stopPropagation()
      open = !open
    }}
  >
    <HugeiconsIcon
      icon={FilterIcon}
      size={16}
      strokeWidth={1.5}
      aria-hidden="true"
    />
  </button>
{/snippet}

{#snippet field()}
  <div {id} hidden={!open} class="list-filter-field">
    <FilterInput
      {label}
      {placeholder}
      bind:value
      bind:input
      onkeydown={event => {
        if (event.key === 'Escape') {
          event.preventDefault()
          event.stopPropagation()
          open = false
          trigger?.focus({ preventScroll: true })
        } else onkeydown?.(event)
      }}
    />
  </div>
{/snippet}

{@render children(toggle, field)}

<style lang="postcss">
  @reference '../../../app.css';
  .list-filter-field {
    @apply shrink-0 px-3 py-2;
  }
</style>

<script lang="ts">
  import type { HTMLInputAttributes } from 'svelte/elements'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { FilterIcon } from '@hugeicons/core-free-icons'

  let {
    id,
    label,
    placeholder = 'Filter',
    value = $bindable(''),
    onkeydown,
    input = $bindable<HTMLInputElement>(),
  }: {
    id?: string
    label: string
    placeholder?: string
    value?: string
    onkeydown?: HTMLInputAttributes['onkeydown']
    input?: HTMLInputElement
  } = $props()
</script>

<div class="relative">
  <HugeiconsIcon
    icon={FilterIcon}
    size={14}
    strokeWidth={1.5}
    class="pointer-events-none absolute left-3.5 top-1/2 -translate-y-1/2 text-subtle"
    aria-hidden="true"
  />
  <input
    bind:this={input}
    {id}
    type="search"
    class="filter-input"
    aria-label={label}
    {placeholder}
    autocomplete="off"
    spellcheck="false"
    {onkeydown}
    bind:value
  />
</div>

<style lang="postcss">
  @reference "../../../app.css";

  .filter-input {
    @apply h-8 min-w-0 w-full rounded-full border-0 pl-9 pr-3.5 py-0 font-sans text-sm text-base-content;
    background-color: var(--color-input);
  }

  .filter-input::placeholder {
    color: var(--color-subtle);
    opacity: 1;
  }
</style>

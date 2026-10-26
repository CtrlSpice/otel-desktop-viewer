<script lang="ts">
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { Tick02Icon, MinusSignIcon } from '@hugeicons/core-free-icons'

  let {
    label,
    checked,
    indeterminate = false,
    disabled = false,
    onchange,
  }: {
    label: string
    checked: boolean
    indeterminate?: boolean
    disabled?: boolean
    onchange: (checked: boolean) => void
  } = $props()
</script>

<span class="selection-checkbox" class:selection-checkbox--disabled={disabled}>
  <input
    type="checkbox"
    aria-label={label}
    {checked}
    {indeterminate}
    {disabled}
    onchange={event => onchange(event.currentTarget.checked)}
  />
  <span class="selection-checkbox__circle" aria-hidden="true">
    {#if checked || indeterminate}<HugeiconsIcon
        icon={indeterminate ? MinusSignIcon : Tick02Icon}
        size={12}
        strokeWidth={1.5}
      />{/if}
  </span>
</span>

<style>
  .selection-checkbox {
    position: relative;
    display: inline-grid;
    width: 1rem;
    height: 1rem;
    flex-shrink: 0;
    color: var(--color-subtle);
    border-radius: 50%;
  }
  .selection-checkbox input {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    margin: 0;
    opacity: 0;
    cursor: inherit;
  }
  .selection-checkbox__circle {
    display: grid;
    place-items: center;
    border: 1px solid var(--color-highlight-high);
    background: var(--color-input);
    border-radius: 50%;
    box-shadow: none;
    pointer-events: none;
  }
  .selection-checkbox:has(input:focus-visible) {
    outline: 2px solid var(--color-primary);
    outline-offset: 2px;
  }
  .selection-checkbox:has(
      input:checked:not(:disabled),
      input:indeterminate:not(:disabled)
    )
    .selection-checkbox__circle {
    border-color: var(--color-primary);
    background: var(--color-primary);
    color: var(--color-primary-content);
  }
  .selection-checkbox--disabled {
    color: var(--color-muted);
  }
</style>

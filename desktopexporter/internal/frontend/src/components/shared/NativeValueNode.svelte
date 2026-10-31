<script lang="ts">
  import type { AttributeValue } from '@/types/api-types'
  import NativeValueNode from './NativeValueNode.svelte'
  import AttributeValueView from './AttributeValue.svelte'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { ArrowRight01Icon, ArrowDown01Icon } from '@hugeicons/core-free-icons'

  let {
    value,
    label,
    depth = 0,
    openDepth = 1,
  }: {
    value: AttributeValue
    label: string
    depth?: number
    openDepth?: number
  } = $props()
  let open = $state(false)
  let mounted = $state(false)
  let limit = $state(100)
  $effect(() => {
    void value
    open = depth < openDepth
    mounted = depth < openDepth
    limit = 100
  })
  $effect(() => {
    if (open) mounted = true
  })
  const container = $derived(value.kind === 'array' || value.kind === 'map')
  const count = $derived(
    value.kind === 'array' || value.kind === 'map'
      ? (value.childCount ?? value.value.length)
      : 0
  )
  const readChild = $derived.by(() => {
    const source = value
    const cache = new Map<
      number,
      { label: string; value: AttributeValue; index: number }
    >()
    function read(index: number) {
      if (source.kind === 'map') {
        const entry = source.childAt
          ? source.childAt(index)
          : source.value[index]!
        return { label: entry.key, value: entry.value, index }
      } else if (source.kind === 'array') {
        return {
          label: String(index),
          value: source.childAt ? source.childAt(index) : source.value[index]!,
          index,
        }
      } else throw new Error('Cannot open children of a scalar')
    }
    return (index: number) => {
      const cached = cache.get(index)
      if (cached) return cached
      const child = read(index)
      cache.set(index, child)
      return child
    }
  })
  const children = $derived.by(() => {
    if (!mounted || (value.kind !== 'array' && value.kind !== 'map')) return []
    return Array.from({ length: Math.min(count, limit) }, (_, index) =>
      readChild(index)
    )
  })
  let fullText = $state(false)
  $effect(() => {
    void value
    fullText = false
  })
  const longText = $derived(
    value.kind === 'string' && value.value.length > 2048
  )
</script>

{#if container}
  <div class="native-value-node">
    <button
      type="button"
      class="native-value-toggle"
      aria-expanded={open}
      aria-label={`${open ? 'Collapse' : 'Expand'} ${label}`}
      onclick={() => (open = !open)}
    >
      <HugeiconsIcon
        icon={open ? ArrowDown01Icon : ArrowRight01Icon}
        size={16}
        strokeWidth={1.5}
        aria-hidden="true"
      />
      <span class="native-value-key">{label}</span>
      <span class="text-subtle"
        >{value.kind === 'map' ? '{' : '['}{open ? '' : `… ${count}`}{open
          ? ''
          : value.kind === 'map'
            ? '}'
            : ']'}</span
      >
    </button>
    {#if mounted}
      <div class="native-value-children" hidden={!open}>
        {#each children as child (child.index)}
          <NativeValueNode
            value={child.value}
            label={child.label}
            depth={depth + 1}
            openDepth={depth === 0 &&
            value.kind === 'array' &&
            child.index === 0
              ? openDepth + 1
              : openDepth}
          />
        {/each}
        {#if count > limit}<button
            type="button"
            class="btn btn-ghost btn-sm font-sans font-normal"
            onclick={() => (limit += 100)}
            >Show next {Math.min(100, count - limit)} of {count}</button
          >{/if}
      </div>
      <span class="text-subtle" hidden={!open}
        >{value.kind === 'map' ? '}' : ']'}</span
      >
    {/if}
  </div>
{:else}
  <div class="native-value-leaf">
    <span class="native-value-key">{label}: </span>
    {#if longText && !fullText && value.kind === 'string'}
      <span class="native-value-text">{value.value.slice(0, 2048)}…</span>
      <button
        type="button"
        class="btn btn-ghost btn-sm font-sans font-normal"
        onclick={() => (fullText = true)}>Show full value of {label}</button
      >
    {:else}<AttributeValueView {value} path={label} />{/if}
  </div>
{/if}

<style lang="postcss">
  @reference '../../app.css';
  .native-value-node,
  .native-value-leaf {
    min-width: 0;
  }
  .native-value-toggle {
    @apply flex max-w-full items-start gap-1 rounded text-left;
  }
  .native-value-toggle:hover {
    background: var(--color-highlight-low);
  }
  .native-value-toggle :global(svg) {
    flex-shrink: 0;
    margin-top: 0.15em;
  }
  .native-value-key {
    color: var(--color-code-ruri);
    overflow-wrap: anywhere;
  }
  .native-value-children {
    padding-left: 1.25rem;
  }
  .native-value-text {
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
</style>

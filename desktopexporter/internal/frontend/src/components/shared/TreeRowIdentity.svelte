<script lang="ts">
  import type { Snippet } from 'svelte'
  import TreeGutter, { type TreeConnectorMeta } from './TreeGutter.svelte'
  import TreeToggle from './TreeToggle.svelte'

  let {
    name,
    depth,
    tree,
    color,
    node,
    subtreeCollapsed = false,
    toggleLabel,
    onToggle,
    children,
    trailing,
    titleClass = '',
    toggleClass = '',
  }: {
    name: string
    depth: number
    tree: TreeConnectorMeta
    color?: string
    node?: Snippet
    subtreeCollapsed?: boolean
    toggleLabel?: string
    onToggle?: () => void
    children: Snippet
    trailing?: Snippet
    titleClass?: string
    toggleClass?: string
  } = $props()
</script>

{#snippet gutter()}<TreeGutter
    {depth}
    {tree}
    {color}
    {node}
    {subtreeCollapsed}
  />{/snippet}

<div class="tree-row-identity">
  {#if tree.childrenCount > 0 && onToggle}
    <TreeToggle
      class={toggleClass}
      label={toggleLabel ??
        `${subtreeCollapsed ? 'Expand' : 'Collapse'} ${name}, ${tree.childrenCount} child ${tree.childrenCount === 1 ? 'span' : 'spans'}`}
      expanded={!subtreeCollapsed}
      onclick={onToggle}
    >
      {@render gutter()}
    </TreeToggle>
  {:else}{@render gutter()}{/if}
  <div class="tree-row-identity__title {titleClass}" title={name}>
    {@render children()}
  </div>
  {@render trailing?.()}
</div>

<style lang="postcss">
  @reference "../../app.css";
  .tree-row-identity {
    @apply flex min-w-0 w-full items-center gap-1;
    min-height: var(--table-row-h);
  }
  .tree-row-identity__title {
    @apply min-w-0 flex-1 text-left text-sm font-normal;
  }
</style>

<script lang="ts">
  import {
    valueTokenTree,
    type ValueNode,
    type ValueToken,
  } from './detail-value'

  let {
    tokens,
    foldable = false,
    path = 'value',
    highlight = true,
  }: {
    tokens: ValueToken[]
    foldable?: boolean
    path?: string
    highlight?: boolean
  } = $props()
  const id = $props.id()
  let nodes = $derived(foldable ? valueTokenTree(tokens, path) : tokens)
  let collapsed = $state<Set<number>>(new Set())
  $effect(() => {
    void tokens
    void path
    collapsed = new Set()
  })
  function toggle(id: number) {
    const next = new Set(collapsed)
    if (next.has(id)) next.delete(id)
    else next.add(id)
    collapsed = next
  }
</script>

{#snippet content(
  nodes: ValueNode[]
)}{#each nodes as node, index (index)}{#if node.kind === 'container'}{@const expanded =
        !collapsed.has(node.id)}{#if node.count}<button
          type="button"
          class="value-syntax__fold"
          aria-label={`${expanded ? 'Collapse' : 'Expand'} ${node.path}`}
          aria-expanded={expanded}
          aria-controls={`${id}-${node.id}`}
          onclick={() => toggle(node.id)}
        ></button>{/if}<span class="detail-code--punctuation"
        >{node.opening.text}</span
      ><span id={`${id}-${node.id}`}
        >{#if expanded}{@render content(node.children)}{:else}<span
            class="value-syntax__summary"
            >… {node.count}
            {node.opening.text === '{'
              ? node.count === 1
                ? 'field'
                : 'fields'
              : node.count === 1
                ? 'item'
                : 'items'}</span
          >{/if}</span
      ><span class="detail-code--punctuation">{node.closing.text}</span
      >{:else}<span class="detail-code--{node.kind}" title={node.type}
        >{node.text}</span
      >{/if}{/each}{/snippet}

<span class:value-syntax={highlight}>{@render content(nodes)}</span>

<style lang="postcss">
  @reference "../../app.css";
  .value-syntax__fold {
    @apply inline-flex cursor-pointer items-center justify-center rounded-sm border-0 bg-transparent p-0 align-baseline text-base-content/60;
    width: 1em;
    height: 1em;
    margin-right: 0.25em;
  }
  .value-syntax__fold::before {
    content: '';
    width: 0;
    height: 0;
    border-top: 0.3em solid transparent;
    border-bottom: 0.3em solid transparent;
    border-left: 0.4em solid currentColor;
  }
  .value-syntax__fold[aria-expanded='true']::before {
    transform: rotate(90deg);
  }
  .value-syntax__summary {
    @apply text-base-content/50;
  }
</style>

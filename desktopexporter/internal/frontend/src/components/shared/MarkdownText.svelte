<script lang="ts">
  import { marked, type Token } from 'marked'

  let { text }: { text: string } = $props()
  const tokens = $derived(marked.lexer(text))

  function linkTarget(value: string): string | undefined {
    try {
      const url = new URL(value)
      return ['https:', 'http:', 'mailto:'].includes(url.protocol)
        ? url.href
        : undefined
    } catch {
      return undefined
    }
  }
</script>

{#snippet content(items: Token[])}
  {#each items as token}
    {#if token.type === 'paragraph'}
      <p>{@render content(token.tokens ?? [])}</p>
    {:else if token.type === 'heading'}
      <div role="heading" aria-level={token.depth}>
        {@render content(token.tokens ?? [])}
      </div>
    {:else if token.type === 'strong'}
      <strong>{@render content(token.tokens ?? [])}</strong>
    {:else if token.type === 'em'}
      <em>{@render content(token.tokens ?? [])}</em>
    {:else if token.type === 'del'}
      <del>{@render content(token.tokens ?? [])}</del>
    {:else if token.type === 'codespan'}
      <code>{token.text}</code>
    {:else if token.type === 'code'}
      <pre><code>{token.text}</code></pre>
    {:else if token.type === 'blockquote'}
      <blockquote>{@render content(token.tokens ?? [])}</blockquote>
    {:else if token.type === 'list'}
      {#if token.ordered}
        <ol start={token.start || 1}>
          {#each token.items as item}<li>
              {@render content(item.tokens)}
            </li>{/each}
        </ol>
      {:else}
        <ul>
          {#each token.items as item}<li>
              {@render content(item.tokens)}
            </li>{/each}
        </ul>
      {/if}
    {:else if token.type === 'table'}
      <div class="markdown-text__table">
        <table>
          <thead
            ><tr
              >{#each token.header as cell}<th
                  >{@render content(cell.tokens)}</th
                >{/each}</tr
            ></thead
          >
          <tbody
            >{#each token.rows as cells}<tr
                >{#each cells as cell}<td>{@render content(cell.tokens)}</td
                  >{/each}</tr
              >{/each}</tbody
          >
        </table>
      </div>
    {:else if token.type === 'link'}
      {@const href = linkTarget(token.href)}
      {#if href}<a {href} rel="noopener noreferrer"
          >{@render content(token.tokens ?? [])}</a
        >
      {:else}{@render content(token.tokens ?? [])}{/if}
    {:else if token.type === 'br'}<br />
    {:else if token.type === 'hr'}<hr />
    {:else if token.type === 'text' && token.tokens}
      {@render content(token.tokens)}
    {:else if token.type !== 'space' && token.type !== 'def'}
      {'text' in token ? token.text : token.raw}
    {/if}
  {/each}
{/snippet}

<!-- Render tokens as escaped Svelte text; source HTML and images are not executed. -->
<div class="markdown-text">{@render content(tokens)}</div>

<style lang="postcss">
  @reference '../../app.css';
  .markdown-text {
    @apply min-w-0 text-sm font-normal text-base-content;
    line-height: 1.5;
    overflow-wrap: anywhere;
  }
  .markdown-text > :first-child {
    margin-block-start: 0;
  }
  .markdown-text > :last-child {
    margin-block-end: 0;
  }
  p,
  pre,
  blockquote,
  ul,
  ol,
  .markdown-text__table {
    margin-block: 0.5rem;
  }
  [role='heading'] {
    @apply text-sm font-normal;
    color: var(--color-subtle);
    margin-block: 0.75rem 0.25rem;
  }
  strong,
  th {
    font-weight: inherit;
  }
  code {
    @apply font-mono text-sm;
  }
  pre {
    @apply bg-base-100 p-2;
    white-space: pre-wrap;
  }
  ul {
    list-style: disc;
    padding-inline-start: 1.5rem;
  }
  ol {
    list-style: decimal;
    padding-inline-start: 1.5rem;
  }
  blockquote {
    @apply border-l border-base-300 pl-3;
  }
  a {
    @apply text-secondary underline;
  }
  .markdown-text__table {
    overflow-x: auto;
  }
  table {
    width: 100%;
  }
  th,
  td {
    @apply p-2 text-left align-top;
  }
  th {
    color: var(--color-subtle);
  }
</style>

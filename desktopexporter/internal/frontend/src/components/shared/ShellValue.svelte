<script lang="ts">
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { ArrowRight01Icon } from '@hugeicons/core-free-icons'
  import ShellSyntax from './ShellSyntax.svelte'
  import ReadonlyCodePanel from './ReadonlyCodePanel.svelte'

  let { text, label }: { text: string; label: string } = $props()
  let expanded = $state(false)
  const contentID = $props.id()
  $effect(() => {
    void text
    expanded = false
  })
</script>

<div class="shell-value">
  <button
    type="button"
    class="shell-value__caret"
    aria-label={`${expanded ? 'Collapse' : 'Expand'} ${label} shell`}
    aria-expanded={expanded}
    aria-controls={contentID}
    onclick={() => (expanded = !expanded)}
    ><HugeiconsIcon
      icon={ArrowRight01Icon}
      size={16}
      strokeWidth={1.5}
      aria-hidden="true"
    /></button
  >
  <div id={contentID} class="min-w-0 flex-1">
    <div class="truncate" hidden={expanded}><ShellSyntax {text} /></div>
    <div hidden={!expanded}>
      <ReadonlyCodePanel
        code={text}
        ariaLabel={`Shell value of ${label}`}
        bare
      />
    </div>
  </div>
</div>

<style lang="postcss">
  @reference '../../app.css';
  .shell-value {
    @apply flex min-w-0 items-start font-mono text-sm;
  }
  .shell-value__caret {
    @apply flex shrink-0 cursor-pointer items-center justify-center rounded-sm border-0 bg-transparent p-0 text-base-content;
    width: var(--waterfall-gutter-col);
    height: 1.5em;
  }
  .shell-value__caret:hover {
    background-color: var(--color-highlight-low);
  }
  .shell-value__caret:focus-visible {
    outline: 2px solid var(--color-primary);
  }
  .shell-value__caret[aria-expanded='true'] :global(svg) {
    transform: rotate(90deg);
  }
</style>

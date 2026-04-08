<script lang="ts">
  import { CheckmarkCircleIcon, CopyIcon } from '@/icons'

  export let code: string;

  let copied = false;

  // Process the code for bash commands
  $: processedCode = processCode(code);

  function processCode(code: string): string {
    return code
      .replace(/^\$ /gm, '<span class="prompt">$</span> ')
      .replace(/^# (.+)$/gm, '<span class="comment"># $1</span>');
  }

  async function copyToClipboard() {
    try {
      await navigator.clipboard.writeText(code);
      copied = true;
      setTimeout(() => {
        copied = false;
      }, 2000);
    } catch (err) {
      console.error('Failed to copy text: ', err);
    }
  }
</script>

<div class="code-block w-full relative">
  <div class="code-header">
    <button
      class="copy-btn"
      onclick={copyToClipboard}
      data-tip={copied ? 'Copied!' : 'Copy to clipboard'}
    >
      {#if copied}
        <CheckmarkCircleIcon class="copy-icon h-4 w-4" aria-hidden="true" />
      {:else}
        <CopyIcon class="copy-icon h-4 w-4" aria-hidden="true" />
      {/if}
    </button>
  </div>
  <pre><code>{@html processedCode}</code></pre>
</div>

<style lang="postcss">
  @reference "../app.css";
  .code-block {
    @apply bg-base-200 rounded-lg border border-base-300;
  }

  .code-header {
    @apply flex justify-end p-2 h-8 relative z-10;
    background: transparent;
  }

  .copy-btn {
    @apply p-1.5 rounded hover:bg-base-300/50 transition-colors tooltip tooltip-left;
  }

  :global(.copy-icon) {
    @apply text-base-content/70;
  }

  .copy-btn:hover :global(.copy-icon) {
    @apply text-base-content;
  }

  .code-block pre {
    @apply text-sm font-mono pl-4 pr-4 pb-4;
    white-space: pre;
    overflow-x: auto;
  }

  .code-block :global(.prompt) {
    @apply text-primary;
    user-select: none;
  }

  .code-block :global(.comment) {
    @apply text-base-content/60;
  }
</style>

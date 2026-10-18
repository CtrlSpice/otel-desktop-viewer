<script lang="ts">
  import type { Snippet } from 'svelte'

  let {
    label,
    icon,
    count,
    countLabel = label.toLowerCase(),
    action,
    children,
  }: {
    label: string
    icon?: Snippet
    count?: number
    countLabel?: string
    action?: Snippet
    children: Snippet
  } = $props()
</script>

{#snippet heading()}
  {@render icon?.()}
  <span>{label}</span>
  {#if count !== undefined}
    <span class="badge-count ml-auto shrink-0">{count} {countLabel}</span>
  {/if}
{/snippet}

{#snippet bubble()}
  {#if count === 0}
    <section
      class="collapse collapse-close min-h-0 bg-base-100"
      aria-label={label}
    >
      <div
        class="collapse-title detail-bubble__heading"
        class:detail-bubble__heading--action={Boolean(action)}
      >
        {@render heading()}
      </div>
    </section>
  {:else}
    <details
      class="collapse detail-bubble__disclosure min-h-0 overflow-y-auto bg-base-100"
      open
    >
      <summary
        class="collapse-title detail-bubble__heading"
        class:detail-bubble__heading--action={Boolean(action)}
      >
        {@render heading()}
      </summary>
      <div class="collapse-content detail-bubble__content">
        {@render children()}
      </div>
    </details>
  {/if}
{/snippet}

{#if action}
  <div class="detail-bubble__with-action">
    {@render bubble()}
    <div class="detail-bubble__action">{@render action()}</div>
  </div>
{:else}
  {@render bubble()}
{/if}

<style lang="postcss">
  @reference "../../app.css";
  .detail-bubble__heading {
    @apply sticky top-0 z-10 flex min-h-10 items-center gap-2 rounded-xl bg-base-300 px-3 py-2.5 text-sm font-medium;
  }
  .detail-bubble__heading--action {
    @apply pe-12;
  }
  .detail-bubble__with-action {
    @apply relative flex min-h-0 flex-col;
  }
  .detail-bubble__with-action .detail-bubble__disclosure {
    @apply flex-1;
  }
  .detail-bubble__action {
    @apply absolute right-3 top-1.5 z-20;
  }
  .detail-bubble__content {
    @apply px-0;
  }
</style>

<script lang="ts">
  import type { Snippet } from 'svelte'
  import { getContext, setContext } from 'svelte'
  import {
    DETAIL_STACK,
    type DetailStackState,
  } from './utils/detail-stack-state.svelte'

  const stack = getContext<DetailStackState | undefined>(DETAIL_STACK)
  // Only this stack's top-level sections participate in its split.
  setContext(DETAIL_STACK, undefined)
  const sectionID = $props.id()
  let expanded = $state(true)
  let disclosure = $state<HTMLDetailsElement>()
  let nextSection = $derived(stack?.next(sectionID))
  let resizable = $derived(stack?.enabled(sectionID) ?? false)

  function register(element: HTMLElement) {
    const unregister = stack?.register({
      id: sectionID,
      label,
      element,
      available: () => count !== 0,
      expanded: () => count !== 0 && expanded,
      setExpanded: open => {
        expanded = open
        if (disclosure) disclosure.open = open
      },
    })
    return { destroy: () => unregister?.() }
  }

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

  $effect(() => stack?.ensureOpen())

  function toggle(event: Event) {
    if (!stack) return
    event.preventDefault()
    stack.setOpen(sectionID, !expanded)
  }
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
      bind:this={disclosure}
      open={expanded}
      ontoggle={event => {
        // Native toggle is queued. Internal resize updates already own this state;
        // only external reveals need to flow back into the stack controller.
        if (event.currentTarget.open === expanded) return
        if (stack) stack.setOpen(sectionID, event.currentTarget.open)
        else expanded = event.currentTarget.open
      }}
    >
      <summary
        class="collapse-title detail-bubble__heading"
        class:detail-bubble__heading--action={Boolean(action)}
        aria-disabled={stack && expanded && !stack.canClose(sectionID)
          ? 'true'
          : undefined}
        onclick={toggle}
      >
        {@render heading()}
      </summary>
      <div class="collapse-content detail-bubble__content">
        {@render children()}
      </div>
    </details>
  {/if}
{/snippet}

{#if action || stack}
  <div
    id={sectionID}
    class:detail-bubble__with-action={Boolean(action)}
    class:detail-bubble--resizable={Boolean(stack)}
    style:flex={stack?.flex(sectionID)}
    use:register
  >
    {@render bubble()}
    {#if action}<div class="detail-bubble__action">{@render action()}</div>{/if}
  </div>
{:else}
  {@render bubble()}
{/if}

{#if stack && nextSection}
  <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
  <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
  <div
    class="col-resize-bar col-resize-bar--row-in-flow detail-bubble__resize"
    class:col-resize-bar--active={stack.dragging === sectionID}
    role="separator"
    aria-orientation="horizontal"
    aria-label={`Resize ${label} and ${nextSection.label} sections`}
    aria-controls={`${sectionID} ${nextSection.id}`}
    aria-disabled={!resizable}
    aria-valuenow={stack.percent(sectionID)}
    aria-valuemin={0}
    aria-valuemax={100}
    tabindex={resizable ? 0 : -1}
    title="Drag to resize or collapse; double-click to reset section sizes"
    onpointerdown={event => stack.start(sectionID, event)}
    onkeydown={event => stack.keydown(sectionID, event)}
    ondblclick={() => {
      if (resizable) stack.reset()
    }}
  >
    <div class="col-resize-bar__line"></div>
  </div>
{/if}

<style lang="postcss">
  @reference "../../app.css";
  .detail-bubble__heading {
    @apply sticky top-0 z-10 flex min-h-10 items-center gap-2 rounded-xl border border-base-300 bg-base-200 text-sm font-medium;
    padding: calc(0.625rem - 1px) calc(0.75rem - 1px);
  }

  .detail-bubble--resizable {
    display: flex;
    flex-direction: column;
    min-height: 2.5rem;
    min-width: 0;
    overflow: hidden;
  }

  .detail-bubble--resizable > .detail-bubble__disclosure {
    flex: 1;
  }

  .detail-bubble__resize[aria-disabled='true'] {
    pointer-events: none;
    cursor: default;
  }

  @media (prefers-reduced-motion: no-preference) {
    .detail-bubble--resizable {
      interpolate-size: allow-keywords;
      transition:
        flex-basis var(--detail-motion-duration) var(--detail-motion-easing),
        flex-grow var(--detail-motion-duration) var(--detail-motion-easing);
    }
  }
  .detail-bubble__heading--action {
    padding-inline-end: calc(3rem - 1px);
  }
  .detail-bubble__with-action:has(
      .detail-bubble__action :global([role='tablist'])
    )
    .detail-bubble__heading--action {
    padding-inline-end: calc(15rem - 1px);
  }
  .detail-bubble__with-action {
    @apply relative flex min-h-0 flex-col;
  }
  .detail-bubble__with-action .detail-bubble__disclosure {
    @apply flex-1;
  }
  .detail-bubble__action {
    @apply absolute right-3 top-1.5 z-20 flex items-center gap-1;
  }
  .detail-bubble__action:has(:global([role='tablist'])) {
    top: 0.5rem;
  }
  .detail-bubble__content {
    @apply px-0;
  }
</style>

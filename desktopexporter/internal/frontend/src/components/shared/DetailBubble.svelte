<script lang="ts">
  import type { Snippet } from 'svelte'
  import { getContext, setContext, untrack } from 'svelte'
  import {
    DETAIL_STACK,
    type DetailStackState,
  } from './utils/detail-stack-state.svelte'

  const stack = getContext<DetailStackState | undefined>(DETAIL_STACK)
  // Only this stack's top-level sections participate in its split.
  setContext(DETAIL_STACK, undefined)
  const sectionID = $props.id()
  let disclosure = $state<HTMLDetailsElement>()
  let previousSection = $derived(stack?.previous(sectionID))
  let suppressClick = false
  let pointerFocus = $state(false)

  function startResize(event: PointerEvent) {
    pointerFocus = true
    suppressClick = false
    if (!resizable || !stack || !previousSection || event.button !== 0) return
    if (
      event.target instanceof Element &&
      event.target.closest('button, a, input, select, textarea, [role="tab"]')
    )
      return
    ;(event.currentTarget as HTMLElement).focus({ preventScroll: true })
    stack.start(previousSection.id, event, () => {
      suppressClick = true
    })
  }

  function resizeKey(event: KeyboardEvent) {
    if (event.target === event.currentTarget) pointerFocus = false
    if (event.target !== event.currentTarget || !resizable || !previousSection)
      return
    stack?.keydown(previousSection.id, event)
  }

  function register(element: HTMLElement) {
    const unregister = stack?.register({
      id: sectionID,
      get label() {
        return label
      },
      get layoutKey() {
        return layoutKey
      },
      element,
      available: () => !disabled && count !== 0,
      expanded: () => !disabled && count !== 0 && expanded,
      setExpanded: open => {
        expanded = open
        if (disclosure) disclosure.open = open
      },
    })
    return { destroy: () => unregister?.() }
  }

  let {
    label,
    layoutKey,
    labelSuffix,
    icon,
    count,
    countLabel = label.toLowerCase(),
    trailing,
    action,
    children,
    initialOpen = true,
    disabled = false,
  }: {
    label: string
    /** Preserve stored section sizing when its visible heading changes. */
    layoutKey?: string
    labelSuffix?: Snippet
    icon?: Snippet
    count?: number
    countLabel?: string
    trailing?: Snippet
    action?: Snippet
    children: Snippet
    initialOpen?: boolean
    disabled?: boolean
  } = $props()
  let expanded = $state(untrack(() => initialOpen))
  let resizable = $derived(
    !disabled &&
      count !== 0 &&
      previousSection &&
      stack?.enabled(previousSection.id)
  )

  $effect(() => stack?.ensureOpen())

  function toggle(event: Event) {
    if (!stack) return
    event.preventDefault()
    const dragged =
      suppressClick && event instanceof MouseEvent && event.detail > 0
    suppressClick = false
    if (dragged) return
    stack.setOpen(sectionID, !expanded)
  }
</script>

{#snippet heading()}
  {@render icon?.()}
  <span>{label}</span>
  {@render labelSuffix?.()}
  {#if count !== undefined}
    <span class="badge-count ml-auto shrink-0">{count} {countLabel}</span>
  {/if}
  {#if trailing}<span class="ml-auto shrink-0">{@render trailing()}</span>{/if}
{/snippet}

{#snippet bubble()}
  {#if disabled || count === 0}
    <section
      class="collapse collapse-close detail-bubble--unavailable min-h-0 bg-transparent"
      aria-label={label}
    >
      <div
        role={disabled ? 'button' : undefined}
        aria-disabled={disabled || undefined}
        class="collapse-title detail-bubble__heading"
        class:detail-bubble__heading--action={Boolean(action)}
        class:detail-bubble__heading--disabled={disabled || count === 0}
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
        class:detail-bubble__heading--resizable={!!resizable}
        class:detail-bubble__heading--pointer-focus={pointerFocus}
        data-resize-before={resizable ? previousSection?.id : undefined}
        title={resizable
          ? 'Drag or use arrow keys to resize; click to expand or collapse; double-click to reset sizes'
          : undefined}
        aria-describedby={resizable ? `${sectionID}-resize-help` : undefined}
        onpointerdown={startResize}
        onkeydown={resizeKey}
        onblur={() => (pointerFocus = false)}
        ondblclick={() => {
          if (resizable) stack?.reset()
        }}
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
    class:detail-bubble--unavailable={disabled || count === 0}
    style:flex={stack?.flex(sectionID)}
    use:register
  >
    {@render bubble()}
    {#if resizable}<span class="sr-only" id={`${sectionID}-resize-help`}
        >Drag to resize sections. Up and Down arrows resize; Shift increases the
        step. Home and End collapse either side.</span
      >{/if}
    {#if action && !disabled}<div class="detail-bubble__action">
        {@render action()}
      </div>{/if}
  </div>
{:else}
  {@render bubble()}
{/if}

<style lang="postcss">
  @reference "../../app.css";
  .detail-bubble__heading {
    @apply sticky top-0 z-10 flex items-center gap-2 rounded-xl border border-base-300 bg-base-200 text-sm font-medium text-(--color-subtle);
    min-height: var(--pane-header-h);
    padding: var(--header-padding) var(--header-padding-inline);
    line-height: 1.5;
  }

  .detail-bubble__heading:focus:not(:focus-visible) {
    outline: none;
  }

  .detail-bubble__heading:focus-visible {
    outline: 2px solid var(--color-primary);
    outline-offset: -3px;
  }

  .detail-bubble__heading--pointer-focus:focus {
    outline: none;
  }

  /* The summary owns focus feedback; an outer disclosure ring is clipped by
     the stack and leaves stray marks at the section corners. */
  .detail-bubble__disclosure {
    outline: none;
  }

  .detail-bubble--resizable {
    display: flex;
    flex-direction: column;
    min-height: var(--pane-header-h);
    min-width: 0;
    overflow: hidden;
  }
  .detail-bubble__heading--disabled {
    background-image: linear-gradient(
      var(--color-highlight-low),
      var(--color-highlight-low)
    );
    color: var(--color-muted);
    cursor: default;
  }
  .detail-bubble--unavailable {
    order: 1;
  }
  .detail-bubble__heading--disabled :global(*) {
    color: inherit;
  }

  .detail-bubble--resizable > .detail-bubble__disclosure {
    flex: 1;
  }

  .detail-bubble__heading--resizable {
    cursor: grab;
    touch-action: none;
    user-select: none;
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
    padding-inline-end: calc(var(--pane-inline-inset) + 2.25rem - 1px);
  }
  .detail-bubble__with-action:has(
      .detail-bubble__action :global([role='tablist'])
    )
    .detail-bubble__heading--action {
    padding-inline-end: calc(var(--detail-tabs-space, 16.5rem) - 1px);
  }
  .detail-bubble__with-action {
    @apply relative flex min-h-0 flex-col;
    container: header / inline-size;
  }
  .detail-bubble__with-action .detail-bubble__disclosure {
    @apply flex-1;
  }
  .detail-bubble__action {
    @apply absolute z-20 flex items-center;
    top: calc(var(--pane-header-h) / 2);
    transform: translateY(-50%);
    right: var(--header-padding-inline);
    gap: var(--header-gap);
  }
  @container header (width < 25rem) {
    .detail-bubble__heading--action {
      --detail-tabs-space: 8.5rem;
    }
  }
  .detail-bubble__content {
    @apply px-0;
  }
</style>

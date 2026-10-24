<script lang="ts">
  import type { Snippet } from 'svelte'
  import { getContext, setContext, tick, untrack } from 'svelte'
  import {
    DETAIL_STACK,
    DETAIL_SECTION,
    type DetailStackState,
  } from './utils/detail-stack-state.svelte'

  const stack = getContext<DetailStackState | undefined>(DETAIL_STACK)
  // Only this stack's top-level sections participate in its split.
  setContext(DETAIL_STACK, undefined)
  const sectionID = $props.id()
  const sectionKey = getContext<(() => string) | undefined>(DETAIL_SECTION)
  const stackID = $derived(sectionKey?.() ?? sectionID)
  let disclosure = $state<HTMLDetailsElement>()
  let previousSection = $derived(stack?.previous(stackID))
  let suppressClick = false
  let pointerFocus = $state(false)
  let actionWidth = $state(0)
  let headingHeight = $state(0)
  let scrollTop = 0
  let disclosureGeneration = 0

  function rememberScroll() {
    // Native collapse and resize clamping must not replace the reading position.
    if (disclosure?.open && expanded && !stack?.dragging)
      scrollTop = disclosure.scrollTop
  }

  function setExpanded(open: boolean) {
    if (expanded === open) {
      if (disclosure) disclosure.open = open
      return
    }
    if (!open) rememberScroll()
    expanded = open
    const element = disclosure
    if (element) element.open = open
    const generation = ++disclosureGeneration
    if (open && element) {
      void tick().then(() => {
        if (
          generation === disclosureGeneration &&
          element === disclosure &&
          element.open
        )
          element.scrollTop = scrollTop
      })
    }
  }

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
      id: stackID,
      get label() {
        return label
      },
      get layoutKey() {
        return layoutKey
      },
      element,
      available: () => !unavailable,
      expanded: () => !unavailable && expanded,
      setExpanded,
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
  const unavailable = $derived(disabled || count === 0)
  let expanded = $state(untrack(() => initialOpen))
  let resizable = $derived(
    !unavailable &&
      expanded &&
      previousSection &&
      stack?.enabled(previousSection.id)
  )

  $effect(() => stack?.ensureOpen())

  function toggle(event: Event) {
    if (!stack) return
    const dragged =
      suppressClick && event instanceof MouseEvent && event.detail > 0
    suppressClick = false
    if (dragged || (expanded && !stack.canClose(stackID)))
      event.preventDefault()
  }
</script>

{#snippet heading()}
  {@render icon?.()}
  <span>{label}</span>
  {@render labelSuffix?.()}
  {#if count !== undefined}
    <span
      class="badge-count ml-auto shrink-0"
      aria-label={`${count.toLocaleString()} ${countLabel}`}
      >[{count.toLocaleString()}]</span
    >
  {/if}
  {#if trailing}<span class="ml-auto shrink-0">{@render trailing()}</span>{/if}
{/snippet}

{#snippet bubble()}
  {#if unavailable}
    <section
      class="collapse collapse-close detail-bubble--unavailable min-h-0 bg-transparent"
      aria-label={label}
    >
      <div
        role="button"
        aria-disabled="true"
        class="collapse-title detail-bubble__heading detail-bubble__heading--disabled"
      >
        {@render heading()}
      </div>
    </section>
  {:else}
    <details
      class="collapse detail-bubble__disclosure min-h-0 overflow-y-auto bg-base-100"
      bind:this={disclosure}
      style:scroll-padding-block-start={headingHeight
        ? `${headingHeight}px`
        : 'var(--pane-header-h)'}
      open={expanded}
      onscroll={rememberScroll}
      ontoggle={event => {
        // Native disclosure owns clicks, keyboard activation and field reveals.
        // Resizing also writes `open`; ignore those already-synchronized toggles.
        if (event.currentTarget.open === expanded) return
        if (stack) stack.setOpen(stackID, event.currentTarget.open)
        else setExpanded(event.currentTarget.open)
      }}
    >
      <summary
        bind:clientHeight={headingHeight}
        class="collapse-title detail-bubble__heading"
        class:detail-bubble__heading--action={Boolean(action)}
        class:detail-bubble__heading--resizable={!!resizable}
        class:detail-bubble__heading--pointer-focus={pointerFocus}
        data-resize-before={resizable ? previousSection?.element.id : undefined}
        title={resizable
          ? 'Drag or use arrow keys to resize; click to collapse; press 0 to reset sizes'
          : undefined}
        aria-describedby={resizable ? `${sectionID}-resize-help` : undefined}
        onpointerdown={startResize}
        onkeydown={resizeKey}
        onblur={() => (pointerFocus = false)}
        aria-disabled={stack && expanded && !stack.canClose(stackID)
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
    class="detail-bubble"
    class:detail-bubble--resizable={Boolean(stack)}
    class:detail-bubble--unavailable={unavailable}
    data-detail-section={stackID}
    data-detail-group={unavailable ? 'inactive' : expanded ? 'open' : 'closed'}
    style:flex={stack?.flex(stackID)}
    style:--detail-action-width={actionWidth ? `${actionWidth}px` : undefined}
    use:register
  >
    {@render bubble()}
    {#if resizable}<span class="sr-only" id={`${sectionID}-resize-help`}
        >Drag to resize open sections. Up and Down arrows resize; Shift
        increases the step. Home and End shrink either side; the top stays open.
        Press 0 to reset sizes.</span
      >{/if}
    {#if action && !unavailable}<div
        class="detail-bubble__action"
        bind:clientWidth={actionWidth}
      >
        {@render action()}
      </div>{/if}
  </div>
{:else}
  {@render bubble()}
{/if}

<style lang="postcss">
  @reference "../../app.css";
  .detail-bubble {
    @apply relative flex min-h-0 flex-col;
    container: header / inline-size;
  }

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
  .detail-bubble__heading--disabled :global(*) {
    color: inherit;
  }

  .detail-bubble__heading--resizable {
    cursor: grab;
    touch-action: none;
    user-select: none;
  }

  .detail-bubble__heading--action {
    padding-inline-end: calc(
      var(--header-padding-inline) + var(--detail-action-width, 2rem) +
        var(--header-gap)
    );
  }
  .detail-bubble > .detail-bubble__disclosure {
    @apply flex-1;
  }
  .detail-bubble__action {
    @apply absolute z-20 flex items-center;
    top: calc(var(--pane-header-h) / 2);
    transform: translateY(-50%);
    right: var(--header-padding-inline);
    gap: var(--header-gap);
  }
  .detail-bubble__content {
    @apply px-0;
  }
</style>

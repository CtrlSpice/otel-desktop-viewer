<script lang="ts">
  import { onDestroy } from 'svelte'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { CheckmarkCircle02Icon, Copy01Icon } from '@hugeicons/core-free-icons'
  import { copyText } from '@/utils/clipboard'

  let {
    text = '',
    loadText,
    label,
    tooltip = 'Copy value',
    tooltipSide = 'left',
    contextual = false,
    grouped = false,
  }: {
    text?: string
    /** Fetch exact source text only when the user requests a copy. */
    loadText?: (signal: AbortSignal) => Promise<string>
    label: string
    tooltip?: string
    tooltipSide?: 'left' | 'right'
    contextual?: boolean
    /** Match the compact joined inspector tabs when used in their header. */
    grouped?: boolean
  } = $props()
  let status = $state('')
  let copied = $state(false)
  let failed = $state(false)
  let pending = $state(false)
  let activeRequest: AbortController | undefined
  let timer: ReturnType<typeof setTimeout> | undefined

  function reset() {
    clearTimeout(timer)
    copied = false
    failed = false
    status = ''
  }
  $effect(() => {
    void text
    void loadText
    void label
    activeRequest?.abort()
    activeRequest = undefined
    pending = false
    reset()
  })
  onDestroy(() => {
    clearTimeout(timer)
    activeRequest?.abort()
  })

  async function copy(event: MouseEvent) {
    event.stopPropagation()
    if (pending) return
    const current = text
    const currentLoader = loadText
    const currentLabel = label
    const controller = new AbortController()
    activeRequest = controller
    pending = true
    reset()
    try {
      const value = currentLoader
        ? await currentLoader(controller.signal)
        : current
      controller.signal.throwIfAborted()
      if (
        text !== current ||
        loadText !== currentLoader ||
        label !== currentLabel
      )
        return
      await copyText(value)
      if (
        controller.signal.aborted ||
        text !== current ||
        loadText !== currentLoader ||
        label !== currentLabel
      )
        return
      copied = true
      status = `Copied ${label}.`
      timer = setTimeout(reset, 1000)
    } catch {
      if (
        !controller.signal.aborted &&
        text === current &&
        loadText === currentLoader &&
        label === currentLabel
      ) {
        failed = true
        status = `Could not copy ${label}.`
      }
    } finally {
      if (activeRequest === controller) {
        activeRequest = undefined
        pending = false
      }
    }
  }
</script>

<button
  type="button"
  class="{grouped
    ? 'btn btn-xs btn-square chart-header-control value-copy--grouped'
    : 'header-icon-button'} value-copy tooltip"
  class:tooltip-left={tooltipSide === 'left'}
  class:tooltip-right={tooltipSide === 'right'}
  class:value-copy--copied={copied}
  class:value-copy--failed={failed}
  class:value-copy--contextual={contextual}
  aria-label="Copy {label}"
  aria-busy={pending}
  aria-disabled={pending}
  data-tip={copied ? 'Copied' : failed ? 'Copy failed' : tooltip}
  onclick={copy}
>
  <HugeiconsIcon
    icon={copied ? CheckmarkCircle02Icon : Copy01Icon}
    size={16}
    strokeWidth={1.5}
    aria-hidden="true"
  />
</button>
<span class="sr-only" role="status">{status}</span>

<style lang="postcss">
  @reference "../../app.css";
  .value-copy--grouped {
    @apply size-6 min-w-6;
    --header-control-height: 1.5rem;
    --header-control-padding: 0;
  }
  .value-copy--copied {
    color: var(--color-accent);
  }
  @media (hover: hover) and (pointer: fine) {
    .value-copy--contextual {
      opacity: 0;
      pointer-events: none;
    }
    :global(.detail-property__card:hover) .value-copy--contextual,
    :global(.detail-property__card:focus-within) .value-copy--contextual,
    .value-copy--contextual:is(
      :focus-visible,
      .value-copy--copied,
      .value-copy--failed
    ) {
      opacity: 1;
      pointer-events: auto;
    }
  }
</style>

<script lang="ts">
  import { onDestroy } from 'svelte'
  import { createRead } from '@/services/reads'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { CheckmarkCircle02Icon, Copy01Icon } from '@hugeicons/core-free-icons'
  import { copyText } from '@/utils/clipboard'

  let {
    text = '',
    loadText,
    sourceKey,
    label,
    tooltip = 'Copy value',
    contextual = false,
    grouped = false,
  }: {
    text?: string
    /** Fetch exact source text only when the user requests a copy. */
    loadText?: (signal: AbortSignal) => Promise<string>
    /** Identity of the displayed source, not a pending selection. */
    sourceKey?: unknown
    label: string
    tooltip?: string
    contextual?: boolean
    /** Match the compact joined inspector tabs when used in their header. */
    grouped?: boolean
  } = $props()
  let status = $state('')
  let copied = $state(false)
  let failed = $state(false)
  const copyRead = createRead<boolean>({ group: null })
  const pending = $derived(copyRead.pending)
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
    void sourceKey
    void label
    copyRead.clear()
    reset()
  })
  onDestroy(() => {
    clearTimeout(timer)
  })

  async function copy(event: MouseEvent) {
    event.stopPropagation()
    if (pending) return
    const current = text
    const currentLoader = loadText
    const currentSource = sourceKey
    const currentLabel = label
    reset()
    await copyRead.load(
      async signal => {
        const value = currentLoader ? await currentLoader(signal) : current
        signal.throwIfAborted()
        if (
          text !== current ||
          loadText !== currentLoader ||
          sourceKey !== currentSource ||
          label !== currentLabel
        )
          return false
        await copyText(value)
        return !(
          text !== current ||
          loadText !== currentLoader ||
          sourceKey !== currentSource ||
          label !== currentLabel
        )
      },
      current => {
        if (!current) return
        copied = true
        status = `Copied ${label}.`
        timer = setTimeout(reset, 1000)
      },
      {
        onError: () => {
          if (
            text === current &&
            loadText === currentLoader &&
            sourceKey === currentSource &&
            label === currentLabel
          ) {
            failed = true
            status = `Could not copy ${label}.`
          }
        },
      }
    )
  }
</script>

<button
  type="button"
  class="{grouped
    ? 'btn btn-xs btn-square chart-header-control value-copy--grouped'
    : 'header-icon-button'} value-copy tooltip tooltip-left"
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
    min-width: var(--control-target-size);
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

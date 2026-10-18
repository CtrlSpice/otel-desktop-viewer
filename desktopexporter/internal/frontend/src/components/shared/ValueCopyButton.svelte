<script lang="ts">
  import { onDestroy } from 'svelte'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { CheckmarkCircle02Icon, Copy01Icon } from '@hugeicons/core-free-icons'
  import { copyText } from '@/utils/clipboard'

  let {
    text,
    label,
    tooltip = 'Copy value',
    contextual = false,
  }: {
    text: string
    label: string
    tooltip?: string
    contextual?: boolean
  } = $props()
  let status = $state('')
  let copied = $state(false)
  let failed = $state(false)
  let timer: ReturnType<typeof setTimeout> | undefined

  function reset() {
    clearTimeout(timer)
    copied = false
    failed = false
    status = ''
  }
  $effect(() => {
    void text
    void label
    reset()
  })
  onDestroy(() => clearTimeout(timer))

  export async function copy(event?: MouseEvent) {
    event?.stopPropagation()
    const current = text
    const currentLabel = label
    reset()
    try {
      await copyText(current)
      if (text !== current || label !== currentLabel) return
      copied = true
      status = `Copied ${label}.`
      timer = setTimeout(reset, 2000)
    } catch {
      if (text === current && label === currentLabel) {
        failed = true
        status = `Could not copy ${label}.`
      }
    }
  }
</script>

<button
  type="button"
  class="header-icon-button value-copy tooltip tooltip-left"
  class:value-copy--copied={copied}
  class:value-copy--failed={failed}
  class:value-copy--contextual={contextual}
  aria-label="Copy {label}"
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

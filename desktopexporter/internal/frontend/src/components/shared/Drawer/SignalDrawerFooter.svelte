<script lang="ts">
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { Delete02Icon } from '@hugeicons/core-free-icons'

  type SignalLabel = 'trace' | 'log' | 'metric'

  interface Props {
    label: SignalLabel
    onDeleteAll: () => void
  }

  let { label, onDeleteAll }: Props = $props()
</script>

<button
  type="button"
  class="btn btn-ghost btn-xs text-error"
  onclick={onDeleteAll}
  aria-label={`Delete all ${label}s`}
>
  <HugeiconsIcon
    icon={Delete02Icon}
    size="1em"
    strokeWidth={1.5}
    class="h-3 w-3"
    aria-hidden="true"
  />
  Delete all
</button>

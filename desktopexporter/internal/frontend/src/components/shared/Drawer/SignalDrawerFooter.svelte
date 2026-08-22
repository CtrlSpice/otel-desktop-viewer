<script lang="ts">
  import { TrashIcon } from '@/icons'

  type SignalLabel = 'trace' | 'log' | 'metric'

  interface Props {
    count: number
    label: SignalLabel
    onDeleteAll: () => void
  }

  let { count, label, onDeleteAll }: Props = $props()

  const PLURAL: Record<SignalLabel, string> = {
    trace: 'traces',
    log: 'logs',
    metric: 'metrics',
  }

  let countLabel = $derived(`${count} ${count === 1 ? label : PLURAL[label]}`)

  let deleteAriaLabel = $derived(`Delete all ${PLURAL[label]}`)
</script>

<div class="flex items-center justify-between">
  <span class="text-xs tabular-nums text-base-content/50">
    {countLabel}
  </span>
  <button
    type="button"
    class="btn btn-ghost btn-xs text-error"
    onclick={onDeleteAll}
    aria-label={deleteAriaLabel}
  >
    <TrashIcon class="h-3 w-3" aria-hidden="true" />
    Delete all
  </button>
</div>

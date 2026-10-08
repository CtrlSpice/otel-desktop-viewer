<script lang="ts">
  import { Cancel01Icon } from '@hugeicons/core-free-icons'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import type { ImportFailure } from '@/types/import-types'
  import { INGESTION_ISSUES_ID } from './IngestionIssues.svelte'

  let {
    failure,
    onViewIssue,
    onDismiss,
  }: {
    failure: ImportFailure
    onViewIssue: (event: MouseEvent) => void
    onDismiss: () => void
  } = $props()
</script>

<div class="toast toast-end toast-bottom z-40 max-w-full">
  <div
    class="alert alert-soft alert-error flex max-w-md gap-3 text-sm text-base-content shadow-sm"
    role="alert"
  >
    <span class="min-w-0 break-words">
      Couldn't import <span class="font-medium">{failure.fileName}</span>.
    </span>
    <a
      class="link shrink-0 whitespace-nowrap"
      href="/#{INGESTION_ISSUES_ID}"
      onclick={onViewIssue}>View issue</a
    >
    <button
      type="button"
      class="btn btn-ghost btn-xs btn-square shrink-0"
      aria-label="Dismiss import failure notice"
      onclick={onDismiss}
    >
      <HugeiconsIcon
        icon={Cancel01Icon}
        size={14}
        strokeWidth={1.5}
        aria-hidden="true"
      />
    </button>
  </div>
</div>

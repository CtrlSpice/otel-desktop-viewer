<script module lang="ts">
  export const INGESTION_ISSUES_ID = 'ingestion-issues'
</script>

<script lang="ts">
  import DisclosureSignalCard from './DisclosureSignalCard.svelte'
  import Identifier from './Identifier.svelte'
  import LogField from '@/components/shared/Detail/DetailField.svelte'
  import { itemHref, SPAN_PARAM } from '@/route'
  import type { Rejection } from '@/types/api-types'
  import type { ImportFailure } from '@/types/import-types'
  import { groupRejections, latestIssueTime } from './ingestion-issues'

  let {
    rejections,
    importFailures,
    rejectionLabel,
    formatRelativeTime,
  }: {
    rejections: readonly Rejection[]
    importFailures: readonly ImportFailure[]
    rejectionLabel: (kind: string) => string
    formatRelativeTime: (timestamp: bigint | null | undefined) => string
  } = $props()

  const rejectionGroups = $derived(groupRejections(rejections))
  const lastImportFailure = $derived(
    latestIssueTime(importFailures.map(failure => failure.occurredAt))
  )
</script>

{#if importFailures.length > 0 || rejections.length > 0}
  <section
    id={INGESTION_ISSUES_ID}
    class="ingestion-issues"
    aria-label="Ingestion issues"
    tabindex="-1"
  >
    {#if importFailures.length > 0}
      <DisclosureSignalCard title="File import issues" wrapTitle>
        {#snippet badge()}
          <span class="issues-count">
            {importFailures.length.toLocaleString()}
            {importFailures.length === 1 ? 'file' : 'files'}
          </span>
        {/snippet}
        {#snippet date()}
          <span class="issues-time"
            >Last seen {formatRelativeTime(lastImportFailure)}</span
          >
        {/snippet}
        <ul class="issues-list issues-details" aria-label="File import issues">
          {#each importFailures as failure}
            <li class="issues-file">
              <p class="issues-file__message">
                <span>{failure.fileName}</span>
                <span aria-hidden="true"> — </span>
                {failure.reason}
              </p>
            </li>
          {/each}
        </ul>
      </DisclosureSignalCard>
    {/if}
    {#each rejectionGroups as group (group.kind)}
      {@const requests = group.kind === 'receiver_recursion_depth'}
      {@const unit = requests ? 'request' : 'record'}
      <DisclosureSignalCard title={rejectionLabel(group.kind)} wrapTitle>
        {#snippet badge()}
          <span class="issues-count">
            {group.occurrences.toLocaleString()}
            {unit}{group.occurrences === 1 ? '' : 's'}
          </span>
        {/snippet}
        {#snippet date()}
          <span class="issues-time"
            >Last seen {formatRelativeTime(group.lastSeen)}</span
          >
        {/snippet}
        <div class="issues-details">
          <table
            class="detail-fields w-full"
            aria-label={rejectionLabel(group.kind)}
          >
            <tbody>
              <LogField
                fieldName={group.signals.length === 1 ? 'signal' : 'signals'}
                fieldType="string"
                showType={false}
                fieldValue={group.signals.join(', ')}
              />
              {#if group.samples.length > 0}
                <LogField
                  fieldName="samples"
                  fieldType="string"
                  showType={false}
                  multiline={true}
                >
                  {#snippet value()}
                    <span class="issues-samples">
                      {#each group.samples as sample (sample.traceID + sample.spanID)}
                        <a
                          class="issues-sample-link link text-sm"
                          href={itemHref('traces', sample.traceID, {
                            [SPAN_PARAM]: sample.spanID,
                          })}
                        >
                          <Identifier value={sample.spanID} />
                        </a>
                      {/each}
                    </span>
                  {/snippet}
                </LogField>
              {/if}
            </tbody>
          </table>
          {#if requests}
            <p class="mb-0 mt-2 text-sm leading-relaxed text-base-content">
              The receiver rejected these requests because their data is nested
              too deeply.
            </p>
            <a
              class="issues-help-link link text-sm"
              href="https://github.com/CtrlSpice/otel-desktop-viewer/blob/main/docs/sdk-configuration.md#deeply-nested-telemetry"
              target="_blank"
              rel="noopener noreferrer">Learn more</a
            >
          {/if}
        </div>
      </DisclosureSignalCard>
    {/each}
  </section>
{/if}

<style lang="postcss">
  @reference '../../app.css';

  .ingestion-issues:focus-visible {
    @apply rounded-lg outline-2 outline-offset-2 outline-primary;
  }

  .ingestion-issues :global(.detail-cell__key),
  .issues-time {
    color: var(--color-base-content);
  }

  .issues-count {
    @apply text-sm tabular-nums text-base-content;
  }

  .issues-details {
    padding: 0 var(--pane-inline-inset) 0.75rem;
  }

  .issues-list {
    @apply m-0 list-none;
  }

  .issues-file + .issues-file {
    @apply mt-2;
  }

  .issues-file__message {
    @apply m-0 break-words text-sm leading-relaxed text-base-content;
    overflow-wrap: anywhere;
  }

  .issues-samples {
    @apply inline-flex flex-wrap gap-x-2 gap-y-0.5;
  }

  .issues-sample-link {
    color: var(--color-primary);
  }

  .issues-sample-link,
  .issues-help-link {
    @apply inline-flex min-h-6 min-w-6 items-center;
  }
</style>

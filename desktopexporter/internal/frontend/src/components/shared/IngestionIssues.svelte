<script module lang="ts">
  export const INGESTION_ISSUES_ID = 'ingestion-issues'
</script>

<script lang="ts">
  import Identifier from './Identifier.svelte'
  import LogField from '@/components/shared/DetailField.svelte'
  import { itemHref, SPAN_PARAM } from '@/route'
  import type { Rejection } from '@/types/api-types'
  import type { ImportFailure } from '@/types/import-types'

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

  const rejectedTotal = $derived(
    rejections.reduce((sum, rejection) => sum + rejection.occurrences, 0)
  )
</script>

{#if importFailures.length > 0 || rejections.length > 0}
  <section
    id={INGESTION_ISSUES_ID}
    class="ingestion-issues p-3"
    aria-label="Ingestion issues"
    tabindex="-1"
  >
    {#if importFailures.length > 0}
      <div class="issues-section">
        <h3 class="issues-heading">
          File import issues
          <span class="text-subtle">
            {importFailures.length}
            {importFailures.length === 1 ? 'file' : 'files'}
          </span>
        </h3>
        <ul class="issues-list" aria-label="File import issues">
          {#each importFailures as failure}
            <li class="issues-file">
              <p class="issues-file__message">
                <span>{failure.fileName}</span>
                <span aria-hidden="true"> — </span>
                {failure.reason}
              </p>
              <p class="issues-time">
                {formatRelativeTime(failure.occurredAt)}
              </p>
            </li>
          {/each}
        </ul>
      </div>
    {/if}
    {#if rejections.length > 0}
      <div class="issues-section">
        <h3 class="issues-heading">
          Rejected telemetry
          <span class="text-subtle">
            {rejectedTotal} records
          </span>
        </h3>
        <table class="detail-fields w-full" aria-label="Rejected telemetry">
          <tbody>
            {#each rejections as rejection (rejection.signal + rejection.kind)}
              <LogField
                fieldName={rejectionLabel(rejection.kind)}
                fieldType={rejection.signal}
                showType={true}
                fieldValue={String(rejection.occurrences)}
              />
              <LogField
                fieldName="last seen"
                fieldType="string"
                showType={false}
                fieldValue={formatRelativeTime(rejection.lastSeen)}
              />
              {#if rejection.samples.length > 0}
                <LogField
                  fieldName="samples"
                  fieldType="string"
                  showType={false}
                  multiline={true}
                >
                  {#snippet value()}
                    <span class="issues-samples">
                      {#each rejection.samples as sample (sample.traceID + sample.spanID)}
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
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </section>
{/if}

<style lang="postcss">
  @reference '../../app.css';

  .ingestion-issues:focus-visible {
    @apply rounded-lg outline-2 outline-offset-2 outline-primary;
  }

  .issues-heading,
  .issues-time {
    color: var(--color-subtle);
  }

  .issues-section + .issues-section {
    @apply mt-3 border-t border-(--color-border) pt-3;
  }

  .issues-heading {
    @apply mb-2 flex items-center justify-between gap-2 text-sm font-normal;
  }

  .issues-list {
    @apply m-0 list-none p-0;
  }

  .issues-file + .issues-file {
    @apply mt-2;
  }

  .issues-file__message {
    @apply m-0 break-words text-sm leading-relaxed text-base-content;
    overflow-wrap: anywhere;
  }

  .issues-time {
    @apply mb-0 mt-0.5 text-sm;
  }

  .issues-samples {
    @apply inline-flex flex-wrap gap-x-2 gap-y-0.5;
  }

  .issues-sample-link {
    color: var(--color-primary);
  }
</style>

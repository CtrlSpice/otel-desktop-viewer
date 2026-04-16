<script lang="ts">
  import type { LogData } from '@/types/api-types'
  import { severityBand } from '@/pages/LogsPage.svelte'
  import { getServiceName } from '@/utils/resource'
  import { formatTimestamp } from '@/utils/time'
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import { TrashIcon } from '@/icons'

  type Props = {
    log: LogData | undefined
    onDelete: (id: string) => void
  }

  let { log, onDelete }: Props = $props()

  let timeContext = getTimeContext()

  let service = $derived(log ? (getServiceName(log.resource) ?? '') : '')
</script>

{#if log}
  <div class="log-detail-panel">
    <div class="log-detail-panel__scroll">
      <table class="detail-fields w-full" aria-label="Log details">
        <thead class="table-header-surface">
          <tr class="table-header-row">
            <th class="table-header-cell table-header-cell--left" colspan="3">
              Log Record
            </th>
          </tr>
        </thead>
        <tbody>
          <tr class="table-row">
            <td class="detail-cell" colspan="2"><span class="detail-cell__key">timestamp:</span> <span class="tabular-nums">{formatTimestamp(log.timestamp, timeContext.timezone, 'nanoseconds')}</span></td>
            <td class="detail-cell--badges"><span class="badge-type">timestamp</span></td>
          </tr>
          <tr class="table-row">
            <td class="detail-cell" colspan="2"><span class="detail-cell__key">observed timestamp:</span> <span class="tabular-nums">{formatTimestamp(log.observedTimestamp, timeContext.timezone, 'nanoseconds')}</span></td>
            <td class="detail-cell--badges"><span class="badge-type">timestamp</span></td>
          </tr>
          <tr class="table-row">
            <td class="detail-cell" colspan="2">
              <span class="detail-cell__key">severity:</span>
              <span class="detail-cell__value">
                {log.severityText || severityBand(log.severityNumber).toUpperCase()} ({log.severityNumber})
              </span>
            </td>
            <td class="detail-cell--badges"><span class="badge-type">enum</span></td>
          </tr>
          <tr class="table-row">
            <td class="detail-cell" colspan="2"><span class="detail-cell__key">service:</span> {service || '—'}</td>
            <td class="detail-cell--badges"><span class="badge-type">string</span></td>
          </tr>
          {#if log.traceID}
            <tr class="table-row">
              <td class="detail-cell" colspan="2"><span class="detail-cell__key">trace id:</span> <a href="/trace/{log.traceID}" class="link link-primary font-mono">{log.traceID}</a></td>
              <td class="detail-cell--badges"><span class="badge-type">string</span></td>
            </tr>
          {/if}
          {#if log.spanID}
            <tr class="table-row">
              <td class="detail-cell" colspan="2"><span class="detail-cell__key">span id:</span> <span class="font-mono">{log.spanID}</span></td>
              <td class="detail-cell--badges"><span class="badge-type">string</span></td>
            </tr>
          {/if}
          {#if log.body}
            <tr class="table-row">
              <td class="log-detail-body__key"><span class="detail-cell__key">body:</span></td>
              <td class="log-detail-body__value">{log.body}</td>
              <td class="detail-cell--badges"><span class="badge-type">{log.bodyType}</span></td>
            </tr>
          {/if}
          {#if log.eventName}
            <tr class="table-row">
              <td class="detail-cell" colspan="2"><span class="detail-cell__key">event name:</span> {log.eventName}</td>
              <td class="detail-cell--badges"><span class="badge-type">string</span></td>
            </tr>
          {/if}
          <tr class="table-row">
            <td class="detail-cell" colspan="2"><span class="detail-cell__key">flags:</span> {log.flags}</td>
            <td class="detail-cell--badges"><span class="badge-type">uint32</span></td>
          </tr>
          {#if log.droppedAttributesCount > 0}
            <tr class="table-row">
              <td class="detail-cell" colspan="2"><span class="detail-cell__key">dropped attributes count:</span> {log.droppedAttributesCount}</td>
              <td class="detail-cell--badges"><span class="badge-type">uint32</span></td>
            </tr>
          {/if}
          {#each log.attributes as attr}
            <tr class="table-row">
              <td class="detail-cell" colspan="2"><span class="detail-cell__key">{attr.key}:</span> {attr.value}</td>
              <td class="detail-cell--badges"><span class="badge-type">{attr.type}</span></td>
            </tr>
          {/each}
          {#each log.resource.attributes as attr}
            <tr class="table-row">
              <td class="detail-cell" colspan="2"><span class="detail-cell__key">{attr.key}:</span> {attr.value}</td>
              <td class="detail-cell--badges"><span class="badge-type">{attr.type}</span> <span class="badge-origin">resource</span></td>
            </tr>
          {/each}
          {#if log.scope.name}
            <tr class="table-row">
              <td class="detail-cell" colspan="2"><span class="detail-cell__key">scope name:</span> {log.scope.name}</td>
              <td class="detail-cell--badges"><span class="badge-type">string</span> <span class="badge-origin">scope</span></td>
            </tr>
          {/if}
          {#if log.scope.version}
            <tr class="table-row">
              <td class="detail-cell" colspan="2"><span class="detail-cell__key">scope version:</span> {log.scope.version}</td>
              <td class="detail-cell--badges"><span class="badge-type">string</span> <span class="badge-origin">scope</span></td>
            </tr>
          {/if}
          {#each log.scope.attributes as attr}
            <tr class="table-row">
              <td class="detail-cell" colspan="2"><span class="detail-cell__key">{attr.key}:</span> {attr.value}</td>
              <td class="detail-cell--badges"><span class="badge-type">{attr.type}</span> <span class="badge-origin">scope</span></td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
    <div class="log-detail-panel__footer">
      <button
        type="button"
        class="btn btn-ghost btn-sm text-error"
        onclick={() => onDelete(log.id)}
        aria-label="Delete this log"
      >
        <TrashIcon class="h-3.5 w-3.5" aria-hidden="true" />
        Delete this log
      </button>
    </div>
  </div>
{:else}
  <div class="log-detail-panel log-detail-panel--empty">
    <p class="text-base-content/40 text-sm">Select a log to view details</p>
  </div>
{/if}

<style lang="postcss">
  @reference "../../app.css";

  .log-detail-panel {
    @apply flex h-full flex-col overflow-hidden;
  }

  .log-detail-panel--empty {
    @apply items-center justify-center;
  }

  .log-detail-panel__scroll {
    @apply flex-1 min-h-0 overflow-y-auto;
  }

  .log-detail-body__key {
    @apply py-1 pl-4 pr-1 text-xs align-top whitespace-nowrap;
    width: 1px;
  }

  .log-detail-body__value {
    @apply py-1 pr-2 text-xs align-top text-base-content;
    white-space: normal;
    overflow-wrap: break-word;
    word-break: break-word;
  }

  .log-detail-panel__footer {
    @apply flex items-center justify-end gap-2 border-t border-base-300/50 px-4 py-2;
  }
</style>

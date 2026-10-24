<script module lang="ts">
  import { GRAPH_INKS } from '@/themes/graph-inks'
  import { colorIdentityHash } from '@/utils/color-slots'

  // Middle-strength official shades stay clear as small marks in Day and Night.
  const valueColors = GRAPH_INKS.flatMap(ink => ink.swatches.slice(2, 5))
  const signalLabels = { traces: 'Traces', metrics: 'Metrics', logs: 'Logs' }
</script>

<script lang="ts">
  import type { Snippet } from 'svelte'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { ArrowRight01Icon } from '@hugeicons/core-free-icons'
  import TreeToggle from '@/components/shared/TreeToggle.svelte'
  import TableValueCell from '@/components/shared/TableValueCell.svelte'
  import DetailNav from '@/components/shared/DetailNav.svelte'
  import { selectableTableRow } from '@/components/shared/utils/selectable-table-row'
  import type {
    AttributeKey,
    DiscoveryOptions,
  } from '@/services/discovery-types'
  import type { AttributeColumn, AttributeSummary } from './attribute-columns'
  import {
    coverageCounts,
    discoverySignals,
    loadAttributeCoverage,
    sortCoverageValues,
    type AttributeCoverage,
    type AttributeValueSort,
    type CoverageValue,
  } from './attribute-coverage'
  import ShellSyntax from '@/components/shared/ShellSyntax.svelte'
  import ValueSyntax from '@/components/shared/ValueSyntax.svelte'
  import ValueCopyButton from '@/components/shared/ValueCopyButton.svelte'
  import {
    attributeValueCopy,
    attributeValueTokens,
    jsonContainerTokens,
  } from '@/components/shared/detail-value'
  import {
    valuePresentation,
    valuePresentationKey,
  } from '@/state/value-presentation.svelte'

  let {
    options,
    name,
    kind,
    heading: attributeHeading,
    sortBy,
    sortDirection,
    columns,
    pinned,
    foundOn,
    selected,
    selectedValue,
    onSelect,
    onSelectValue,
    onSummary,
    hidden = false,
  }: {
    options: Omit<DiscoveryOptions, 'kind'>
    name: string
    kind: string
    heading: Snippet
    sortBy: AttributeValueSort
    sortDirection: 'asc' | 'desc'
    columns: AttributeColumn[]
    pinned: Record<string, number | undefined>
    foundOn: AttributeKey['foundOn']
    selected: boolean
    selectedValue?: string
    onSelect: () => void
    onSelectValue: (value: CoverageValue) => void
    onSummary: (summary: AttributeSummary) => void
    hidden?: boolean
  } = $props()
  let coverage = $state<AttributeCoverage>()
  const kindCoverage = $derived(coverage?.kinds.get(kind))
  const PAGE_SIZE = 25
  let offset = $state(0)
  const values = $derived(
    sortCoverageValues(
      coverage?.values.filter(item => item.value.kind === kind) ?? [],
      sortBy,
      sortDirection
    )
  )
  const page = $derived(values.slice(offset, offset + PAGE_SIZE))
  const pageIndex = $derived(Math.floor(offset / PAGE_SIZE))
  const pageCount = $derived(Math.max(1, Math.ceil(values.length / PAGE_SIZE)))
  const presentationKey = $derived(
    valuePresentationKey(options.ownerType ?? '', 'attribute', name)
  )
  const signals = $derived(
    discoverySignals
      .flatMap(signal =>
        foundOn.some(source => source.signal === signal)
          ? [signalLabels[signal]]
          : []
      )
      .join(' · ')
  )
  let loading = $state(false)
  let error = $state('')
  const headingID = $props.id()
  let expanded = $state(false)
  const canExpand = $derived(values.length > 0)
  $effect.pre(() => {
    void sortBy
    void sortDirection
    offset = 0
  })
  $effect(() => {
    const controller = new AbortController()
    loading = true
    error = ''
    void loadAttributeCoverage({ ...options, key: name }, controller.signal)
      .then(result => {
        if (controller.signal.aborted) return
        coverage = result
        const group = result.kinds.get(kind)
        onSummary({
          count: group?.count ?? 0,
          values: result.values.filter(item => item.value.kind === kind).length,
          counts: group?.counts ?? { traces: 0, metrics: 0, logs: 0 },
        })
      })
      .catch(reason => {
        if (!controller.signal.aborted) error = String(reason)
      })
      .finally(() => {
        if (!controller.signal.aborted) loading = false
      })
    return () => controller.abort()
  })
</script>

{#snippet heading()}
  <div id={headingID} class="flex min-w-0 items-center gap-2 text-subtle">
    {#if canExpand}
      <TreeToggle
        label={`${expanded ? 'Collapse' : 'Expand'} ${name} · ${options.ownerType} values`}
        {expanded}
        onclick={() => (expanded = !expanded)}
      >
        <HugeiconsIcon
          icon={ArrowRight01Icon}
          size={16}
          strokeWidth={1.5}
          class={expanded ? 'rotate-90' : ''}
          aria-hidden="true"
        />
      </TreeToggle>
    {:else}<span class="w-4 shrink-0" aria-hidden="true"></span>{/if}
    {@render attributeHeading()}
  </div>
{/snippet}

{#if !hidden}
  <tr
    class="table-row"
    class:table-row--selected={selected && !selectedValue}
    use:selectableTableRow={{ selected: selected && !selectedValue, onSelect }}
    data-selectable-row
    data-attribute-heading={name}
    data-attribute-source={options.ownerType}
  >
    {#each columns as column (column.id)}
      <svelte:element
        this={column.id === 'name' ? 'th' : 'td'}
        scope={column.id === 'name' ? 'row' : undefined}
        class="table-cell font-normal"
        data-attribute-column={column.id}
        class:text-left={['name', 'type', 'source', 'signals'].includes(
          column.id
        )}
        class:text-right={!['name', 'type', 'source', 'signals'].includes(
          column.id
        )}
        class:tabular-nums={[
          'values',
          'count',
          'traces',
          'metrics',
          'logs',
        ].includes(column.id)}
        class:table-cell--pinned={pinned[column.id] !== undefined}
        style:left={pinned[column.id] === undefined
          ? undefined
          : `${pinned[column.id]}px`}
      >
        {#if column.id === 'name'}{@render heading()}
        {:else if column.id === 'type'}<span class="text-secondary" title={kind}
            >{kind}</span
          >
        {:else if column.id === 'source'}<span
            class="text-secondary"
            title={options.ownerType}>{options.ownerType}</span
          >
        {:else if column.id === 'signals'}{signals}
        {:else if column.id === 'values'}{coverage
            ? values.length.toLocaleString()
            : '—'}
        {:else if column.id === 'count'}
          <span
            title={kindCoverage
              ? `${values.length.toLocaleString()} distinct ${values.length === 1 ? 'value' : 'values'} · ${coverageCounts(kindCoverage.counts)}`
              : undefined}
            >{coverage
              ? (kindCoverage?.count ?? 0).toLocaleString()
              : '—'}</span
          >
        {:else if column.id === 'traces' || column.id === 'metrics' || column.id === 'logs'}
          {coverage
            ? (kindCoverage?.counts[column.id] ?? 0).toLocaleString()
            : '—'}
        {/if}
      </svelte:element>
    {/each}
  </tr>

  {#if error || values.length === 0}
    <tr
      ><td class="table-cell" colspan={columns.length}
        ><div class="pl-6">
          {#if error}<span role="alert" class="text-papaya">{error}</span>
          {:else}<span class="text-subtle" role="status"
              >{loading ? 'Loading values…' : 'No retained values.'}</span
            >{/if}
        </div></td
      ></tr
    >
  {:else}
    {#each page as item (item.identity)}
      {@const text = attributeValueCopy(item.value)}
      {@const color =
        valueColors[
          colorIdentityHash(
            JSON.stringify([options.ownerType, name, item.identity])
          ) % valueColors.length
        ]!.color}
      {@const tokens =
        item.value.kind === 'string'
          ? jsonContainerTokens(item.value.value, true)
          : item.value.kind === 'array' || item.value.kind === 'map'
            ? attributeValueTokens(item.value, true)
            : null}
      <tr
        class="table-row"
        class:table-row--selected={selected && selectedValue === item.identity}
        use:selectableTableRow={{
          selected: selected && selectedValue === item.identity,
          onSelect: () => onSelectValue(item),
        }}
        hidden={!expanded}
        data-selectable-row
        data-attribute-key={name}
        data-attribute-source={options.ownerType}
        data-value-identity={item.identity}
        aria-describedby={headingID}
        aria-busy={loading}
      >
        {#each columns as column (column.id)}
          <td
            class="table-cell"
            data-attribute-column={column.id}
            class:text-left={['name', 'type', 'source', 'signals'].includes(
              column.id
            )}
            class:text-right={!['name', 'type', 'source', 'signals'].includes(
              column.id
            )}
            class:tabular-nums={[
              'count',
              'traces',
              'metrics',
              'logs',
              'frequency',
            ].includes(column.id)}
            class:table-cell--pinned={pinned[column.id] !== undefined}
            style:left={pinned[column.id] === undefined
              ? undefined
              : `${pinned[column.id]}px`}
          >
            {#if column.id === 'name'}
              <div class="flex min-w-0 items-center gap-2 pl-6">
                <div class="min-w-0 w-1/3 max-w-48 shrink-0">
                  <TableValueCell
                    label={`${options.ownerType} attribute: ${name}`}
                    {text}
                    inset={false}
                    onInspect={() => onSelectValue(item)}
                  >
                    {#if item.value.kind === 'string' && valuePresentation.isShell(presentationKey)}<ShellSyntax
                        text={item.value.value}
                      />
                    {:else if tokens}<ValueSyntax {tokens} />{:else}{text === ''
                        ? '""'
                        : text}{/if}
                  </TableValueCell>
                </div>
                <span class="flex h-2 min-w-2 flex-1" aria-hidden="true"
                  ><span
                    class="attribute-frequency-bar h-2 rounded-full"
                    style:width={`max(0.5rem, ${item.relativeFrequency * 100}%)`}
                    style:background-color={color}
                  ></span></span
                >
              </div>
            {:else if column.id === 'type'}<span
                class="text-secondary"
                title={item.value.kind}>{item.value.kind}</span
              >
            {:else if column.id === 'source'}<span
                class="text-secondary"
                title={options.ownerType}>{options.ownerType}</span
              >
            {:else if column.id === 'signals'}{discoverySignals
                .flatMap(signal =>
                  item.counts[signal] > 0 ? [signalLabels[signal]] : []
                )
                .join(' · ')}
            {:else if column.id === 'count'}<span
                title={coverageCounts(item.counts)}
                >{item.count.toLocaleString()}</span
              >
            {:else if column.id === 'traces' || column.id === 'metrics' || column.id === 'logs'}{item.counts[
                column.id
              ].toLocaleString()}
            {:else if column.id === 'frequency'}
              <span
                class="text-subtle"
                title={`${coverageCounts(item.counts, coverage?.denominators)} · Of records carrying this attribute`}
                aria-label={`${item.count.toLocaleString()} / ${item.denominator.toLocaleString()} records · ${(item.relativeFrequency * 100).toLocaleString(undefined, { maximumFractionDigits: 1 })}%`}
                >[{(item.relativeFrequency * 100).toLocaleString(undefined, {
                  maximumFractionDigits: 1,
                })}%]</span
              >
            {:else if column.id === 'copy'}<ValueCopyButton
                {text}
                label={`${name} value`}
                tooltip="Copy"
              />{/if}
          </td>
        {/each}
      </tr>
    {/each}
    {#if pageCount > 1}
      <tr hidden={!expanded}
        ><td class="table-cell" colspan={columns.length}
          ><div class="flex justify-center">
            <DetailNav
              index={pageIndex}
              total={pageCount}
              label="page"
              onFirst={() => (offset = 0)}
              onPrev={() => (offset -= PAGE_SIZE)}
              onNext={() => (offset += PAGE_SIZE)}
              onLast={() => (offset = (pageCount - 1) * PAGE_SIZE)}
            />
          </div></td
        ></tr
      >
    {/if}
  {/if}
{/if}

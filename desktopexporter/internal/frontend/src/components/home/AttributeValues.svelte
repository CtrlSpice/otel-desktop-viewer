<script module lang="ts">
  import { GRAPH_INKS } from '@/themes/graph-inks'
  // Middle-strength official shades stay clear as small marks in Day and Night.
  const valueColors = GRAPH_INKS.flatMap(ink => ink.swatches.slice(2, 5))
</script>

<script lang="ts">
  import LoadingStatus from '@/components/shared/LoadingStatus.svelte'
  import type { Snippet } from 'svelte'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { ArrowRight01Icon } from '@hugeicons/core-free-icons'
  import TreeToggle from '@/components/shared/TreeToggle.svelte'
  import TableValueCell from '@/components/shared/TableValueCell.svelte'
  import DetailNav from '@/components/shared/DetailNav.svelte'
  import { selectableTableRow } from '@/components/shared/utils/selectable-table-row'
  import type { DiscoveryOptions } from '@/services/discovery-types'
  import {
    coverageCounts,
    loadAttributeCoverage,
    type AttributeCoverage,
    type AttributeValueSort,
    type CoverageValue,
  } from './attribute-coverage'
  import ShellSyntax from '@/components/shared/ShellSyntax.svelte'
  import ValueSyntax from '@/components/shared/ValueSyntax.svelte'
  import ValueCopyButton from '@/components/shared/ValueCopyButton.svelte'
  import {
    attributeValueCopy,
    compactValueTokens,
    jsonContainerTokens,
  } from '@/components/shared/detail-value'
  import { telemetryAPI } from '@/services/telemetry-service'
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
    pinnedLeft,
    selected,
    selectedValue,
    onSelect,
    onSelectValue,
    onCount,
    expanded,
    onExpandedChange,
    hidden = false,
  }: {
    options: Omit<DiscoveryOptions, 'kind'>
    name: string
    kind: string
    heading: Snippet
    sortBy: AttributeValueSort
    sortDirection: 'asc' | 'desc'
    pinnedLeft?: number
    selected: boolean
    selectedValue?: string
    onSelect: () => void
    onSelectValue: (value: CoverageValue) => void
    onCount: (count: number) => void
    expanded: boolean
    onExpandedChange: (expanded: boolean) => void
    hidden?: boolean
  } = $props()
  let coverage = $state<AttributeCoverage>()
  const kindCoverage = $derived(coverage?.kinds.get(kind))
  const PAGE_SIZE = 25
  const limit = $derived(expanded && !hidden ? PAGE_SIZE : 0)
  type Navigation = { offset: number; previous: number[] }
  let navigation = $state.raw<Navigation>({ offset: 0, previous: [] })
  let displayedNavigation = $state.raw<Navigation>({ offset: 0, previous: [] })
  const values = $derived(coverage?.values ?? [])
  const page = $derived(values)
  const presentationKey = $derived(
    valuePresentationKey(options.ownerType ?? '', 'attribute', name)
  )
  let loading = $state(false)
  let requestKey = $state.raw<AbortSignal>()
  let error = $state('')
  async function copyValue(item: CoverageValue, signal: AbortSignal) {
    try {
      return attributeValueCopy(
        await telemetryAPI.getHomeAttributeValue(item.valueRef, signal)
      )
    } catch (reason) {
      if (!signal.aborted) error = String(reason)
      throw reason
    }
  }
  const headingID = $props.id()
  const canExpand = $derived(coverage?.total !== 0)
  $effect.pre(() => {
    void sortBy
    void sortDirection
    void options
    void name
    void kind
    navigation = { offset: 0, previous: [] }
  })
  $effect(() => {
    const selectedNavigation = navigation
    const controller = new AbortController()
    requestKey = controller.signal
    loading = true
    error = ''
    void loadAttributeCoverage(
      {
        ...options,
        key: name,
        valueKind: kind,
        offset: selectedNavigation.offset,
        limit,
        sortBy,
        sortDirection,
        locale: navigator.language,
      },
      controller.signal
    )
      .then(result => {
        if (controller.signal.aborted) return
        coverage = result
        displayedNavigation = selectedNavigation
        onCount(result.kinds.get(kind)?.count ?? 0)
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
        onclick={() => onExpandedChange(!expanded)}
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
    <th
      scope="row"
      class="table-cell text-left font-normal"
      data-attribute-column="name"
      class:table-cell--pinned={pinnedLeft !== undefined}
      style:left={pinnedLeft === undefined ? undefined : `${pinnedLeft}px`}
    >
      {@render heading()}
    </th>
    <td
      class="table-cell text-right tabular-nums"
      data-attribute-column="count"
    >
      <span
        title={kindCoverage
          ? `${coverage?.total.toLocaleString()} distinct ${coverage?.total === 1 ? 'value' : 'values'} · ${coverageCounts(kindCoverage.counts)}`
          : undefined}
        >{coverage ? (kindCoverage?.count ?? 0).toLocaleString() : '—'}</span
      >
    </td>
    <td class="table-cell text-right" data-attribute-column="frequency"></td>
    <td class="table-cell" data-attribute-column="copy"></td>
  </tr>
  {#if error || loading || (expanded && values.length === 0)}
    <tr
      ><td class="table-cell" colspan="4"
        ><div class="pl-6">
          {#if error}<span role="alert" class="text-papaya">{error}</span>
          {/if}
          <LoadingStatus
            pending={loading}
            label="Loading values…"
            {requestKey}
          />
          {#if !loading && !error && expanded && values.length === 0}<span
              class="text-subtle"
              role="status">No retained values.</span
            >{/if}
        </div></td
      ></tr
    >
  {/if}
  {#if values.length > 0}
    {#each page as item (item.identity)}
      {@const text = item.preview}
      {@const color = valueColors[item.colorHash % valueColors.length]!.color}
      {@const tokens =
        item.kind === 'string'
          ? jsonContainerTokens(text, true)
          : item.kind === 'array' || item.kind === 'map'
            ? compactValueTokens(text, true)
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
        <td
          class="table-cell text-left"
          data-attribute-column="name"
          class:table-cell--pinned={pinnedLeft !== undefined}
          style:left={pinnedLeft === undefined ? undefined : `${pinnedLeft}px`}
        >
          <div class="flex min-w-0 items-center gap-2 pl-6">
            <div class="min-w-0 w-1/3 max-w-48 shrink-0">
              <TableValueCell
                label={`${options.ownerType} attribute: ${name}`}
                {text}
                inset={false}
                onInspect={() => onSelectValue(item)}
                loadText={async signal =>
                  attributeValueCopy(
                    await telemetryAPI.getHomeAttributeValue(
                      item.valueRef,
                      signal
                    )
                  )}
              >
                {#if item.kind === 'string' && valuePresentation.isShell(presentationKey)}<ShellSyntax
                    {text}
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
        </td>
        <td
          class="table-cell text-right tabular-nums"
          data-attribute-column="count"
          ><span title={coverageCounts(item.counts)}
            >{item.count.toLocaleString()}</span
          ></td
        >
        <td
          class="table-cell text-right tabular-nums"
          data-attribute-column="frequency"
        >
          <span
            class="text-subtle"
            title={`${coverageCounts(item.counts, coverage?.denominators)} · Of records carrying this attribute`}
            aria-label={`${item.count.toLocaleString()} / ${item.denominator.toLocaleString()} records · ${(item.relativeFrequency * 100).toLocaleString(undefined, { maximumFractionDigits: 1 })}%`}
            >[{(item.relativeFrequency * 100).toLocaleString(undefined, {
              maximumFractionDigits: 1,
            })}%]</span
          >
        </td>
        <td class="table-cell text-right" data-attribute-column="copy"
          ><ValueCopyButton
            loadText={signal => copyValue(item, signal)}
            label={`${name} value`}
            tooltip="Copy"
          /></td
        >
      </tr>
    {/each}
    {#if displayedNavigation.offset > 0 || coverage?.truncated}
      <tr hidden={!expanded}
        ><td class="table-cell" colspan="4"
          ><div class="flex justify-center">
            <DetailNav
              index={displayedNavigation.previous.length}
              total={undefined}
              hasNext={!!coverage?.truncated && values.length > 0}
              busy={loading}
              label="page"
              onFirst={() => (navigation = { offset: 0, previous: [] })}
              onPrev={() =>
                (navigation = {
                  offset: displayedNavigation.previous.at(-1) ?? 0,
                  previous: displayedNavigation.previous.slice(0, -1),
                })}
              onNext={() =>
                (navigation = {
                  offset: displayedNavigation.offset + values.length,
                  previous: [
                    ...displayedNavigation.previous,
                    displayedNavigation.offset,
                  ],
                })}
            />
          </div></td
        ></tr
      >
    {/if}
  {/if}
{/if}

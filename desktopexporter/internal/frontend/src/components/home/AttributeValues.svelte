<script module lang="ts">
  import { GRAPH_INKS } from '@/themes/graph-inks'
  import { colorIdentityHash } from '@/utils/color-slots'

  // Middle-strength official shades stay clear as small marks in Day and Night.
  const valueColors = GRAPH_INKS.flatMap(ink => ink.swatches.slice(2, 5))
</script>

<script lang="ts">
  import type { Snippet } from 'svelte'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { ArrowRight01Icon } from '@hugeicons/core-free-icons'
  import TreeToggle from '@/components/shared/TreeToggle.svelte'
  import TableValueCell from '@/components/shared/TableValueCell.svelte'
  import DetailNav from '@/components/shared/DetailNav.svelte'
  import type { DiscoveryOptions } from '@/services/discovery-types'
  import {
    coverageCounts,
    loadAttributeCoverage,
    sortCoverageValues,
    type AttributeCoverage,
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
    pinnedLeft,
    selected,
    selectedValue,
    onSelectValue,
    onCount,
  }: {
    options: Omit<DiscoveryOptions, 'kind'>
    name: string
    kind: string
    heading: Snippet
    sortBy: 'name' | 'frequency'
    sortDirection: 'asc' | 'desc'
    pinnedLeft?: number
    selected: boolean
    selectedValue?: string
    onSelectValue: (value: CoverageValue) => void
    onCount: (count: number) => void
  } = $props()
  let coverage = $state<AttributeCoverage>()
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
    const request = {
      ...options,
      key: name,
    }
    loading = true
    error = ''
    void loadAttributeCoverage(request, controller.signal)
      .then(result => {
        if (controller.signal.aborted) return
        coverage = result
        onCount(result.denominator)
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
        ><HugeiconsIcon
          icon={ArrowRight01Icon}
          size={16}
          strokeWidth={1.5}
          class={expanded ? 'rotate-90' : ''}
          aria-hidden="true"
        /></TreeToggle
      >
    {:else}<span class="w-4 shrink-0" aria-hidden="true"></span>{/if}
    {@render attributeHeading()}
  </div>
{/snippet}

<tr
  class="table-row"
  class:table-row--selected={selected && !selectedValue}
  data-attribute-heading={name}
  data-attribute-source={options.ownerType}
>
  <th
    scope="row"
    class="table-cell text-left font-normal"
    class:table-cell--pinned={pinnedLeft !== undefined}
    style:left={pinnedLeft === undefined ? undefined : `${pinnedLeft}px`}
  >
    {@render heading()}
  </th>
  <td class="table-cell text-right tabular-nums">
    <span title={coverage ? coverageCounts(coverage.denominators) : undefined}
      >{coverage ? coverage.denominator.toLocaleString() : '—'}</span
    >
  </td>
  <td class="table-cell text-right"></td>
  <td class="table-cell"></td>
</tr>

{#if error || values.length === 0}
  <tr>
    <td class="table-cell" colspan="4">
      <div class="pl-6">
        {#if error}<span role="alert">{error}</span>
        {:else}<span class="text-subtle" role="status"
            >{loading ? 'Loading values…' : 'No retained values.'}</span
          >{/if}
      </div>
    </td>
  </tr>
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
      hidden={!expanded}
      data-attribute-key={name}
      data-attribute-source={options.ownerType}
      data-value-identity={item.identity}
      aria-describedby={headingID}
      aria-busy={loading}
    >
      <td
        class="table-cell text-left"
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
            >
              {#if item.value.kind === 'string' && valuePresentation.isShell(presentationKey)}<ShellSyntax
                  text={item.value.value}
                />
              {:else if tokens}<ValueSyntax {tokens} />
              {:else}{text === '' ? '""' : text}{/if}
            </TableValueCell>
          </div>
          {#if item.value.kind !== kind}<span class="shrink-0 text-secondary"
              >{item.value.kind}</span
            >{/if}
          <span class="flex h-2 min-w-2 flex-1" aria-hidden="true">
            <span
              class="attribute-frequency-bar h-2 rounded-full"
              style:width={`max(0.5rem, ${item.relativeFrequency * 100}%)`}
              style:background-color={color}
            ></span>
          </span>
        </div>
      </td>
      <td class="table-cell text-right tabular-nums">
        <span title={coverageCounts(item.counts)}
          >{item.count.toLocaleString()}</span
        >
      </td>
      <td class="table-cell text-right text-subtle tabular-nums">
        <span
          title={`${coverageCounts(item.counts, coverage?.denominators)} · Of records carrying this attribute`}
          aria-label={`${item.count.toLocaleString()} / ${item.denominator.toLocaleString()} records · ${(item.relativeFrequency * 100).toLocaleString(undefined, { maximumFractionDigits: 1 })}%`}
        >
          [{(item.relativeFrequency * 100).toLocaleString(undefined, {
            maximumFractionDigits: 1,
          })}%]
        </span>
      </td>
      <td class="table-cell text-right">
        <ValueCopyButton {text} label={`${name} value`} tooltip="Copy" />
      </td>
    </tr>
  {/each}
  {#if pageCount > 1}
    <tr hidden={!expanded}>
      <td class="table-cell" colspan="4">
        <div class="flex justify-center">
          <DetailNav
            index={pageIndex}
            total={pageCount}
            label="page"
            onFirst={() => (offset = 0)}
            onPrev={() => (offset -= PAGE_SIZE)}
            onNext={() => (offset += PAGE_SIZE)}
            onLast={() => (offset = (pageCount - 1) * PAGE_SIZE)}
          />
        </div>
      </td>
    </tr>
  {/if}
{/if}

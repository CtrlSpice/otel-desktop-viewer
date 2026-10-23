<script lang="ts">
  import BreakdownSegment from '@/components/shared/BreakdownSegment.svelte'
  import { attributeValueCopy } from '@/components/shared/detail-value'
  import { coverageCounts, type AttributeCoverage } from './attribute-coverage'
  let {
    coverage,
    kind,
    colors,
  }: {
    coverage: AttributeCoverage
    kind: string
    colors: Record<string, string>
  } = $props()
  let width = $state(0)
  const parts = $derived.by(() => {
    const sorted = coverage.values
      .filter(item => item.value.kind === kind)
      .sort((a, b) => b.count - a.count || a.identity.localeCompare(b.identity))
    const kept = sorted.filter(
      (item, index) =>
        sorted.length === 1 ||
        (index < 8 && item.relativeFrequency * width >= 8)
    )
    const other = sorted.filter(item => !kept.includes(item))
    const otherTypes = coverage.values.filter(item => item.value.kind !== kind)
    return [
      ...kept.map(item => ({
        key: item.identity,
        label: attributeValueCopy(item.value) || '""',
        count: item.count,
        color: colors[item.identity],
        detail: coverageCounts(item.counts, coverage.denominators),
      })),
      ...(other.length
        ? [
            {
              key: 'other-values',
              label: `Other · ${other.length} values`,
              count: other.reduce((sum, item) => sum + item.count, 0),
              color: 'var(--color-subtle)',
              detail: 'All values remain in the expandable list.',
            },
          ]
        : []),
      ...(otherTypes.length
        ? [
            {
              key: 'other-types',
              label: 'Other types',
              count: otherTypes.reduce((sum, item) => sum + item.count, 0),
              color: 'var(--color-neutral)',
              detail:
                'Values listed under the same attribute with their received types.',
            },
          ]
        : []),
    ]
  })
</script>

<div class="flex min-w-0 items-center" bind:clientWidth={width}>
  {#if coverage.overlapping}
    <span
      class="truncate text-rp-subtle"
      title="A record carries more than one value. Expand for each value’s independent percentage."
      >Overlapping values</span
    >
  {:else if parts.length === 1 && coverage.values.length === 1}
    <BreakdownSegment
      part={parts[0]}
      unit="record"
      relativeFrequency={1}
      single
      detail={parts[0].detail}
    />
  {:else}
    <div
      class="attribute-coverage-bar flex h-2 w-full gap-0.5"
      role="group"
      aria-label="Attribute value coverage"
    >
      {#each parts as part (part.key)}
        <BreakdownSegment
          {part}
          unit="record"
          relativeFrequency={part.count / coverage.denominator}
          detail={part.detail}
        />
      {/each}
    </div>
  {/if}
</div>

<style>
  .attribute-coverage-bar :global(.breakdown-segment) {
    min-width: 0;
  }
</style>

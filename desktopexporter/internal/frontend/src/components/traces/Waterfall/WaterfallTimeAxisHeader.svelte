<script module lang="ts">
  export type TimeAxisTick = { offsetPercent: number; label: string }
  export type TimeAxisGuide = { offsetPercent: number; major: boolean }

  export type TimeAxisResult = {
    unit: string
    intervalNs: bigint
    ticks: TimeAxisTick[]
  }

  type UnitDef = { name: string; ns: bigint }

  const UNITS: UnitDef[] = [
    { name: 'ns', ns: 1n },
    { name: 'μs', ns: 1_000n },
    { name: 'ms', ns: 1_000_000n },
    { name: 's', ns: 1_000_000_000n },
    { name: 'min', ns: 60_000_000_000n },
  ]

  const NICE_MULTIPLIERS = [1n, 2n, 5n]

  const MAX_AXIS_TICKS = 100

  function collectTickPositions(
    durationNs: bigint,
    stepNs: bigint,
    maxTicks: number
  ): bigint[] {
    const go = (acc: readonly bigint[], pos: bigint): bigint[] => {
      if (pos > durationNs) return [...acc]
      const extended = [...acc, pos]
      if (extended.length >= maxTicks) return extended
      return go(extended, pos + stepNs)
    }
    return go([], 0n)
  }

  // Pick the largest unit whose size fits within the given nanosecond value.
  // e.g. 1,200,000,000ns → "s", 500,000ns → "μs", 42ns → "ns".
  function pickUnit(ns: bigint): UnitDef {
    return (
      UNITS.findLast(u => ns >= u.ns) ??
      [...UNITS].reverse().find(u => ns >= u.ns) ??
      UNITS[0]
    )
  }

  export type WaterfallAxis = {
    layoutDurationNs: bigint
    unit: string
    intervalNs: bigint
    ticks: TimeAxisTick[]
    guides: TimeAxisGuide[]
  }

  // Round a raw interval to a "nice" human-friendly step size.
  //
  // Given a raw interval in nanoseconds and a unit to think in (e.g. ms),
  // converts to that unit, finds the nearest nice number (1, 2, 5 × 10^n),
  // and converts back to nanoseconds.
  //
  // Example: rawIntervalNs=200,000,000 (200ms), unitNs=1,000,000 (ms)
  //   → rawInUnit=200, magnitude=100, candidates=[100,200,500,...], best=200
  //   → returns 200,000,000ns
  function niceStep(rawIntervalNs: bigint, unitNs: bigint): bigint {
    const rawInUnit = Math.max(Number(rawIntervalNs) / Number(unitNs), 1)
    const magnitude = 10 ** Math.floor(Math.log10(rawInUnit))
    const candidates = [
      magnitude,
      ...NICE_MULTIPLIERS.flatMap(m => {
        const n = Number(m)
        return [n * magnitude, n * magnitude * 10]
      }),
    ]
    const best = candidates.reduce((acc, cand) =>
      Math.abs(rawInUnit - cand) < Math.abs(rawInUnit - acc) ? cand : acc
    )
    const stepNs = BigInt(Math.round(best)) * unitNs
    return stepNs < 1n ? 1n : stepNs
  }

  const decimalsForWhole = (whole: bigint): 0 | 1 | 2 =>
    whole < 10n ? 2 : whole < 100n ? 1 : 0

  function formatTickValue(ns: bigint, unitDef: UnitDef): string {
    const unitNs = unitDef.ns
    const whole = ns / unitNs
    const rem = ns % unitNs
    const suffix = unitDef.name

    if (rem === 0n) {
      return `${whole}${suffix}`
    }

    const decimals = decimalsForWhole(whole)

    if (decimals === 0) {
      const rounded = (ns + unitNs / 2n) / unitNs
      return `${rounded}${suffix}`
    }

    const scale = 10n ** BigInt(decimals)
    const totalScaled = (ns * scale + unitNs / 2n) / unitNs
    const wholePart = totalScaled / scale
    const fracPart = totalScaled % scale
    const fracStr = fracPart.toString().padStart(decimals, '0')
    return `${wholePart}.${fracStr}${suffix}`
  }

  function offsetPercent(pos: bigint, durationNs: bigint): number {
    return Number((pos * 10000n) / durationNs) / 100
  }

  function tickAt(
    pos: bigint,
    durationNs: bigint,
    unit: UnitDef
  ): TimeAxisTick {
    return {
      offsetPercent: offsetPercent(pos, durationNs),
      label: formatTickValue(pos, unit),
    }
  }

  function gridGuides(durationNs: bigint, intervalNs: bigint): TimeAxisGuide[] {
    // Quarter-interval guides, using fewer divisions at whole-nanosecond limits.
    const divisions =
      intervalNs % 4n === 0n ? 4n : intervalNs % 2n === 0n ? 2n : 1n
    return collectTickPositions(
      durationNs,
      intervalNs / divisions,
      MAX_AXIS_TICKS * Number(divisions)
    ).map(pos => ({
      offsetPercent: offsetPercent(pos, durationNs),
      major: pos % intervalNs === 0n,
    }))
  }

  // Two-pass unit selection for human-friendly tick labels.
  //
  // The naive approach picks the display unit from the trace duration:
  // a 1.2s trace picks "s", then tries to compute a nice step of 0.2s.
  // But niceStep works in whole-unit multiples, so 0.2 rounds to 0 → breaks.
  //
  // Instead we do two passes:
  //
  //   Pass 1 — pick a "computation unit" from the raw interval (duration / tickCount).
  //            This ensures niceStep always works with whole numbers.
  //            e.g. 1.2s trace → rawInterval=200ms → stepUnit="ms" → niceStep=200ms ✓
  //
  //   Pass 2 — pick the "display unit" from the computed nice step.
  //            If niceStep rounded up across a unit boundary, the labels follow.
  //            e.g. 5.5s trace → rawInterval=917ms → niceStep=1000ms=1s → display="s" ✓
  //
  // This handles every unit boundary uniformly: ns↔μs, μs↔ms, ms↔s, s↔min.

  /** Time axis for the waterfall ruler. Uses the actual trace duration as extent. */
  export function waterfallTimeAxis(
    traceDurationNs: bigint,
    targetTickCount: number
  ): WaterfallAxis {
    if (traceDurationNs <= 0n) {
      return {
        layoutDurationNs: traceDurationNs,
        unit: 'ns',
        intervalNs: 1n,
        ticks: [{ offsetPercent: 0, label: '0ns' }],
        guides: [{ offsetPercent: 0, major: true }],
      }
    }

    const rawInterval = traceDurationNs / BigInt(Math.max(targetTickCount, 1))

    // Pass 1: compute the step in the raw interval's natural unit
    const stepUnit = pickUnit(rawInterval)
    const intervalNs = niceStep(rawInterval, stepUnit.ns)

    // Pass 2: derive the display unit from the actual step
    const displayUnit = pickUnit(intervalNs)

    const positions = collectTickPositions(
      traceDurationNs,
      intervalNs,
      MAX_AXIS_TICKS
    )
    const ticks = positions.map(pos =>
      tickAt(pos, traceDurationNs, displayUnit)
    )

    return {
      layoutDurationNs: traceDurationNs,
      unit: displayUnit.name,
      intervalNs,
      ticks,
      guides: gridGuides(traceDurationNs, intervalNs),
    }
  }

  /** Generic nice time axis (same tick math, no waterfall end padding). */
  export function niceTimeAxis(
    durationNs: bigint,
    targetTickCount: number
  ): TimeAxisResult {
    if (durationNs <= 0n) {
      return {
        unit: 'ns',
        intervalNs: 1n,
        ticks: [{ offsetPercent: 0, label: '0ns' }],
      }
    }

    const rawInterval = durationNs / BigInt(Math.max(targetTickCount, 1))
    const stepUnit = pickUnit(rawInterval)
    const intervalNs = niceStep(rawInterval, stepUnit.ns)
    const displayUnit = pickUnit(intervalNs)

    const positions = collectTickPositions(
      durationNs,
      intervalNs,
      MAX_AXIS_TICKS
    )
    const ticks = positions.map(pos => tickAt(pos, durationNs, displayUnit))

    return { unit: displayUnit.name, intervalNs, ticks }
  }
</script>

<script lang="ts">
  import TableColumnHeading from '@/components/shared/TableColumnHeading.svelte'
  import { columnLabel, type SizedWaterfallColumn } from './waterfall-columns'
  import {
    DURATION_GUTTER_REM,
    OUTSIDE_RECORD_SLOT_REM,
    TIMELINE_LEFT_INSET_REM,
  } from './timeline-markers'

  type Props = {
    traceDurationNs: bigint
    targetTickCount?: number
    tickLabelWidth?: number
    spanColWidth: number
    rootChildCount?: number
    optionalColumns?: SizedWaterfallColumn[]
  }

  let {
    traceDurationNs,
    targetTickCount = 6,
    tickLabelWidth = 80,
    spanColWidth,
    rootChildCount = 0,
    optionalColumns = [],
  }: Props = $props()

  let axis = $derived(waterfallTimeAxis(traceDurationNs, targetTickCount))
</script>

<tr
  class="waterfall-time-axis-header"
  style:--tick-label-w="{tickLabelWidth}px"
  style:--timeline-left-inset="{TIMELINE_LEFT_INSET_REM}rem"
  style:--duration-gutter="{DURATION_GUTTER_REM}rem"
  style:--outside-record-slot="{OUTSIDE_RECORD_SLOT_REM}rem"
>
  <th
    scope="col"
    class="waterfall-time-axis-header__th-label waterfall-time-axis-header__th-span"
    class:waterfall-time-axis-header__th-span--parent={rootChildCount > 0}
    style:width="{spanColWidth}px"
    aria-label="Span name"
  >
    <span class="flex min-w-0 items-center gap-2">
      {#if rootChildCount > 0}
        <span class="waterfall-time-axis-header__count-spacer invisible shrink-0 font-normal tabular-nums" aria-hidden="true"
          >{rootChildCount}</span
        >
      {/if}
      <TableColumnHeading label="name" source="Span" signal="Span" />
    </span>
  </th>
  {#each optionalColumns as column (column.id)}
    <th
      scope="col"
      class="waterfall-time-axis-header__th-label waterfall-time-axis-header__th-attribute"
      style:width="{column.width}px"
      title={columnLabel(column)}
      aria-label={columnLabel(column)}
      data-column-id={column.id}
    >
      <TableColumnHeading
        label={column.key || '(empty key)'}
        source={column.source}
        signal="Span"
      />
    </th>
  {/each}
  <th scope="col" class="waterfall-time-axis-header__th-ruler">
    <div class="waterfall-time-axis-header__ruler">
      {#each axis.ticks as tick}
        <div
          class="waterfall-time-axis-header__tick"
          style:left="{tick.offsetPercent}%"
        >
          <span class="waterfall-time-axis-header__tick-label"
            >{tick.label}</span
          >
        </div>
      {/each}
      {#each axis.guides as guide}
        <span
          class="waterfall-time-axis-header__tick-line dot-guide-vertical"
          class:dot-guide-vertical--minor={!guide.major}
          style:left="{guide.offsetPercent}%"
          aria-hidden="true"
        ></span>
      {/each}
    </div>
  </th>
</tr>

<style lang="postcss">
  @reference "../../../app.css";
  .waterfall-time-axis-header {
    --primary-label-center: 50%;
    height: var(--table-header-h);
  }

  .waterfall-time-axis-header__th-label {
    @apply relative py-2 align-middle text-left text-sm font-semibold tracking-normal;
    color: var(--table-heading-color);
  }

  .waterfall-time-axis-header__th-span {
    padding-left: calc(
      0.5rem + var(--waterfall-gutter-col) + var(--waterfall-gutter-tail) + 0.25rem
    );
  }

  .waterfall-time-axis-header__th-span--parent {
    padding-left: calc(0.5rem + var(--waterfall-gutter-col));
  }

  .waterfall-time-axis-header__count-spacer {
    width: var(--waterfall-gutter-col);
  }

  .waterfall-time-axis-header__th-attribute {
    padding-inline: var(--table-cell-inline-padding, 0.5rem);
  }

  .waterfall-time-axis-header__th-ruler {
    @apply relative min-w-[12rem] align-middle text-xs tracking-normal;
    color: var(--table-heading-color);
    padding-left: var(--timeline-left-inset);
    padding-right: calc(var(--duration-gutter) + var(--outside-record-slot));
  }

  .waterfall-time-axis-header__ruler {
    @apply absolute bottom-0 top-0 min-w-0 overflow-visible;
    left: var(--timeline-left-inset);
    right: calc(var(--duration-gutter) + var(--outside-record-slot));
  }

  .waterfall-time-axis-header__tick {
    @apply absolute bottom-0 h-full;
  }

  .waterfall-time-axis-header__tick-label {
    @apply absolute text-sm font-semibold tracking-normal text-center;
    top: var(--primary-label-center);
    width: var(--tick-label-w);
    margin-left: calc(var(--tick-label-w) / -2);
    transform: translateY(-50%);
  }

  .waterfall-time-axis-header__tick-line {
    @apply absolute bottom-0;
    left: 0;
    top: calc(var(--primary-label-center) + 0.5lh);
    transform: translateX(-50%);
  }
</style>

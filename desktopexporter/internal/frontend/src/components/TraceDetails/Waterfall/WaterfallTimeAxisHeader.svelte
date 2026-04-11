<script module lang="ts">
  export type TimeAxisTick = { offsetPercent: number; label: string }

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

  function tickAt(
    pos: bigint,
    durationNs: bigint,
    unit: UnitDef
  ): TimeAxisTick {
    return {
      offsetPercent: Number((pos * 10000n) / durationNs) / 100,
      label: formatTickValue(pos, unit),
    }
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
  type Props = {
    traceDurationNs: bigint
    targetTickCount?: number
    tickLabelWidth?: number
    spanColWidth: number
    serviceColWidth: number
    onResizeSpanCol: (width: number) => void
    onResizeServiceCol: (width: number) => void
  }

  let {
    traceDurationNs,
    targetTickCount = 6,
    tickLabelWidth = 80,
    spanColWidth,
    serviceColWidth,
    onResizeSpanCol,
    onResizeServiceCol,
  }: Props = $props()

  let axis = $derived(waterfallTimeAxis(traceDurationNs, targetTickCount))

  function startResize(
    currentWidth: number,
    onResize: (width: number) => void,
    e: PointerEvent
  ) {
    const startX = e.clientX
    const startWidth = currentWidth
    const target = e.currentTarget as HTMLElement
    target.setPointerCapture(e.pointerId)

    function onMove(ev: PointerEvent) {
      onResize(startWidth + (ev.clientX - startX))
    }

    function onUp() {
      target.removeEventListener('pointermove', onMove)
      target.removeEventListener('pointerup', onUp)
    }

    target.addEventListener('pointermove', onMove)
    target.addEventListener('pointerup', onUp)
  }
</script>

<tr class="waterfall-time-axis-header" style:--tick-label-w="{tickLabelWidth}px">
  <th
    scope="col"
    class="waterfall-time-axis-header__th-label waterfall-time-axis-header__th-span"
    style:width="{spanColWidth}px"
  >
    Span
    <div
      class="resize-handle"
      role="separator"
      aria-orientation="vertical"
      onpointerdown={e => startResize(spanColWidth, onResizeSpanCol, e)}
    ></div>
  </th>
  <th
    scope="col"
    class="waterfall-time-axis-header__th-label waterfall-time-axis-header__th-service"
    style:width="{serviceColWidth}px"
  >
    Service
    <div
      class="resize-handle"
      role="separator"
      aria-orientation="vertical"
      onpointerdown={e => startResize(serviceColWidth, onResizeServiceCol, e)}
    ></div>
  </th>
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
          <span class="waterfall-time-axis-header__tick-line" aria-hidden="true"
          ></span>
        </div>
      {/each}
    </div>
  </th>
</tr>

<style lang="postcss">
  @reference "../../../app.css";
  .waterfall-time-axis-header {
    height: var(--table-header-h);
  }

  .waterfall-time-axis-header__th-label {
    @apply relative align-middle text-left text-xs font-semibold tracking-normal text-base-content/55;
  }

  .waterfall-time-axis-header__th-span {
    /* Row inset (8px) + gutter(26px) + gap-1(4px) = 38px */
    padding-left: 38px;
  }

  .waterfall-time-axis-header__th-service {
    @apply pl-2 pr-1;
  }

  .waterfall-time-axis-header__th-ruler {
    @apply relative min-w-[12rem] align-middle text-xs tracking-normal text-base-content/55;
    padding-left: 1.25rem;
    padding-right: 1.75rem;
  }

  .waterfall-time-axis-header__ruler {
    @apply absolute bottom-0 top-0 min-w-0 overflow-visible;
    left: 1.25rem;
    right: 1.75rem;
  }

  .waterfall-time-axis-header__tick {
    @apply absolute bottom-0 h-full;
  }

  .waterfall-time-axis-header__tick-label {
    @apply absolute top-1/2 text-xs tracking-normal text-base-content/55 text-center;
    width: var(--tick-label-w);
    margin-left: calc(var(--tick-label-w) / -2);
    transform: translateY(-50%);
  }

  .waterfall-time-axis-header__tick-line {
    @apply absolute bottom-0 w-px bg-base-content/20;
    left: 0;
    top: calc(50% + 10px);
    transform: translateX(-50%);
  }

  .resize-handle {
    @apply absolute top-0 bottom-0 flex items-center justify-center cursor-col-resize;
    right: -3px;
    width: 7px;
    z-index: 2;
  }
</style>

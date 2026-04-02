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

  function pickUnit(durationNs: bigint): UnitDef {
    return (
      UNITS.findLast(u => durationNs >= u.ns) ??
      [...UNITS].reverse().find(u => durationNs >= u.ns) ??
      UNITS[0]
    )
  }

  // Pads the layout duration to the next tick boundary + one extra step.
  // Commented out: we now use the actual trace duration as the axis extent,
  // with the last label showing the real duration.
  //
  // function layoutSpanNs(traceDurationNs: bigint, stepNs: bigint): bigint {
  //   if (traceDurationNs <= 0n) return traceDurationNs
  //   const ceil = ((traceDurationNs + stepNs - 1n) / stepNs) * stepNs
  //   return ceil === traceDurationNs ? ceil + stepNs : ceil
  // }

  export type WaterfallAxis = {
    layoutDurationNs: bigint
    unit: string
    intervalNs: bigint
    ticks: TimeAxisTick[]
  }

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

    const unit = pickUnit(traceDurationNs)
    const rawInterval = traceDurationNs / BigInt(Math.max(targetTickCount, 1))
    const intervalNs = niceStep(rawInterval, unit.ns)

    const positions = collectTickPositions(
      traceDurationNs,
      intervalNs,
      MAX_AXIS_TICKS
    )
    const ticks = positions.map(pos => tickAt(pos, traceDurationNs, unit))

    return {
      layoutDurationNs: traceDurationNs,
      unit: unit.name,
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

    const unit = pickUnit(durationNs)
    const rawInterval = durationNs / BigInt(Math.max(targetTickCount, 1))
    const intervalNs = niceStep(rawInterval, unit.ns)
    const positions = collectTickPositions(
      durationNs,
      intervalNs,
      MAX_AXIS_TICKS
    )
    const ticks = positions.map(pos => tickAt(pos, durationNs, unit))

    return { unit: unit.name, intervalNs, ticks }
  }
</script>

<script lang="ts">
  type Props = {
    traceDurationNs: bigint
    targetTickCount?: number
    spanColWidth: number
    serviceColWidth: number
    onResizeSpanCol: (width: number) => void
    onResizeServiceCol: (width: number) => void
  }

  let {
    traceDurationNs,
    targetTickCount = 6,
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

<tr class="waterfall-time-axis-header">
  <th
    scope="col"
    class="waterfall-time-axis-header__th-label waterfall-time-axis-header__th-span"
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
          <div class="waterfall-time-axis-header__tick-line"></div>
        </div>
      {/each}
    </div>
  </th>
</tr>

<style lang="postcss">
  .waterfall-time-axis-header__th-label {
    @apply relative align-bottom pb-1 text-left text-xs font-medium text-base-content/70;
  }

  .waterfall-time-axis-header__th-span {
    /* Align with root span name: gutter(26px) + gap-1(4px) = 30px */
    padding-left: 30px;
  }

  .waterfall-time-axis-header__th-service {
    @apply pl-2 pr-1;
  }

  .waterfall-time-axis-header__th-ruler {
    @apply relative min-w-[12rem] h-8 px-4 py-0 align-bottom;
  }

  .waterfall-time-axis-header__ruler {
    @apply absolute bottom-0 top-0 min-w-0 overflow-visible;
    left: 16px;
    right: 16px;
  }

  .waterfall-time-axis-header__tick {
    @apply absolute bottom-0 h-full;
  }

  .waterfall-time-axis-header__tick-label {
    @apply absolute bottom-1 text-[10px] text-base-content/50 font-mono whitespace-nowrap;
    transform: translateX(-50%);
  }

  .waterfall-time-axis-header__tick-line {
    @apply absolute bottom-0 w-px h-1.5 bg-base-300;
  }

  .resize-handle {
    @apply absolute top-0 bottom-0 cursor-col-resize;
    right: -3px;
    width: 7px;
    z-index: 2;
  }

  .resize-handle::after {
    content: '';
    @apply absolute top-1 bottom-1 left-1/2 -translate-x-1/2 w-px bg-base-content/10 transition-all duration-100;
  }

  .resize-handle:hover::after {
    @apply w-0.5 bg-primary/40 top-0 bottom-0 rounded-full;
  }

  .resize-handle:active::after {
    @apply w-0.5 bg-primary/60 top-0 bottom-0 rounded-full;
  }
</style>

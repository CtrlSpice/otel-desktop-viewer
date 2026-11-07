<script lang="ts">
  import { sparklineGeometry } from '@/components/metrics/utils/sparkline-geometry'
  /*
   * Sparkline: a single non-interactive SVG polyline scaled to a fixed
   * box. Built by hand rather than via LayerChart because we render one
   * per timeseries row -- often dozens, occasionally hundreds -- and
   * the chart library's per-instance overhead (scales, tooltip
   * plumbing, hover handlers) costs more than the visual we get back
   * at 18px tall. This component does the bare minimum:
   *
   *   - Compute a single SVG path string from {date, value}[] points.
   *   - Normalize x linearly across the box width by time, y linearly
   *     across the box height by min..max value (with a small constant
   *     pad so a flat series doesn't collapse to a single horizontal
   *     line on the bottom edge).
   *   - Stroke with the provided colour. No fill, no markers, no axes,
   *     no grid, no tooltip, no cursor, no pointer events.
   *
   * The caller decides the points (raw / rate / etc.) and the colour
   * (series colour when checked, neutral otherwise); this component is
   * deliberately ignorant of aggregation view, visibility, and theming.
   */

  type Point = { date: Date; value: number; breakBefore?: true }

  type Props = {
    points: readonly Point[]
    color: string
    width?: number
    height?: number
    /** Stroke width in px. Defaults to 1.5; bump to 2 for emphasis,
     *  drop to 1 for very dense lists. */
    strokeWidth?: number
  }

  let {
    points,
    color,
    width = 128,
    height = 18,
    strokeWidth = 1.5,
  }: Props = $props()
  const clipID = $props.id()

  const geometry = $derived(
    sparklineGeometry(points, width, height, strokeWidth)
  )
</script>

<svg
  class="sparkline"
  {width}
  {height}
  viewBox="0 0 {width} {height}"
  aria-hidden="true"
  focusable="false"
>
  {#if geometry.path}
    <defs>
      <clipPath id={clipID} clipPathUnits="userSpaceOnUse">
        {#each geometry.clips as clip}
          <rect x={clip.x} width={clip.width} y="0" {height} />
        {/each}
      </clipPath>
    </defs>
    <path
      d={geometry.path}
      clip-path={`url(#${clipID})`}
      fill="none"
      stroke={color}
      stroke-width={strokeWidth}
      stroke-linecap="round"
      stroke-linejoin="round"
      vector-effect="non-scaling-stroke"
    />
  {/if}
</svg>

<style lang="postcss">
  .sparkline {
    display: block;
    pointer-events: none;
    flex-shrink: 0;
  }
</style>

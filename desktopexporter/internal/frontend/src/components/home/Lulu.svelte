<script module lang="ts">
  import sprite from '@/assets/images/lulu-pixel.txt?raw'
  import inks from '@/themes/graph-inks.json'
  import heatmapPalette from '@/themes/heatmap-palette.json'

  // Pixel Lulu, CC0-1.0; source and character credits are in README.md.
  // Keep the source grid intact; combine pixels into one SVG path per colour.
  // Use the fixed Iris → Love → Rose progression, never the Day reversal.
  const ramp = heatmapPalette.ramps['12'].night
  const colors = {
    o: ramp[0],
    s: ramp[8],
    b: ramp[11],
    g: ramp[5],
    G: ramp[4],
    e: ramp[0],
    w: inks.inks['fuyu-gaki'][7],
  }
  const rows = sprite.trim().split('\n')
  const paths = Object.entries(colors).map(([pixel, color]) => ({
    color,
    d: rows
      .flatMap((row, y) =>
        Array.from(row, (value, x) =>
          value === pixel ? `M${x} ${y}h1v1h-1z` : ''
        )
      )
      .join(''),
  }))
</script>

<svg
  class="lulu"
  viewBox="0 4 32 27"
  role="img"
  aria-label="Lulu the axolotl"
  xmlns="http://www.w3.org/2000/svg"
>
  {#each paths as path}
    <path d={path.d} style:fill={path.color} />
  {/each}
</svg>

<style>
  .lulu {
    display: block;
    width: 100%;
    height: 100%;
    shape-rendering: crispEdges;
    stroke: none;
  }
</style>

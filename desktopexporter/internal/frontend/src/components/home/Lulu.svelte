<script module lang="ts">
  import sprite from '@/assets/images/lulu-pixel.txt?raw'
  import inks from '@/themes/graph-inks.json'

  // Pixel Lulu, CC0-1.0; source and character credits are in README.md.
  // Keep the source grid intact; combine pixels into one SVG path per colour.
  const skin = `color-mix(in srgb, ${inks.inks['edo-murasaki'][5]} 50%, ${inks.inks['edo-murasaki'][6]})`
  const colors = {
    o: inks.inks['edo-murasaki'][1],
    s: inks.inks['edo-murasaki'][4],
    b: skin,
    g: `color-mix(in srgb, ${inks.inks.bishamonten[3]} 75%, ${skin})`,
    G: `color-mix(in srgb, ${inks.inks.bishamonten[2]} 75%, ${skin})`,
    e: inks.inks['edo-murasaki'][1],
    w: inks.inks['edo-murasaki'][7],
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

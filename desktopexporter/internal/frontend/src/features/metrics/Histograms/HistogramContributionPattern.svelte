<script lang="ts">
  let { id, color }: { id: string; color: string } = $props()
</script>

<defs>
  <pattern
    {id}
    width="6"
    height="6"
    patternUnits="userSpaceOnUse"
    style:--contribution-color={color}
  >
    <path d="M-1,1 l2,-2 M0,6 l6,-6 M5,7 l2,-2" />
  </pattern>
</defs>

<style>
  path {
    fill: none;
    stroke: color-mix(
      in oklab,
      var(--contribution-color) 70%,
      var(--color-base-content)
    );
    stroke-width: 1;
  }
</style>

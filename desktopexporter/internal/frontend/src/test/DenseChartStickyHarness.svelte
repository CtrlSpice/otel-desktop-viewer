<script module lang="ts">
  export { mount } from 'svelte'
</script>

<script lang="ts">
  import ChartSticky from '@/components/metrics/Charts/ChartSticky.svelte'
  import ChartSelectionLegend from '@/components/metrics/Charts/ChartSelectionLegend.svelte'

  let selections = $state(0)
  const rows = Array.from({ length: 22 }, (_, index) => ({
    key: `series-${index}`,
    label: `Series ${index + 1}`,
    valueText: String(index + 1),
    color: 'var(--color-primary)',
  }))
</script>

<div class="metric-chart-view dense-chart-fixture">
  <button type="button" onclick={() => selections++}
    >Plot selections: {selections}</button
  >
  <ChartSticky anchor={{ x: 300, y: 140 }}>
    <ChartSelectionLegend timestamp="2023-11-14, 14:13:20.000" {rows} />
  </ChartSticky>
</div>

<style>
  .dense-chart-fixture {
    position: fixed;
    top: 100px;
    left: 50px;
    width: 600px;
    height: 280px;
    z-index: 1000;
    background: var(--color-base-100);
  }
  button {
    position: absolute;
    inset: 0;
  }
</style>

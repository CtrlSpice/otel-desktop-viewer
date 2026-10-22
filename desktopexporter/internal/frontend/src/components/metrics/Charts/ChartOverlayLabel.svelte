<script lang="ts">
  let {
    label,
    active,
    onclick,
    description,
  }: {
    label: string
    active: boolean
    onclick: () => void
    description?: string
  } = $props()
</script>

<button
  type="button"
  class="chart-overlay-label"
  aria-label={`Emphasize ${label} overlay`}
  aria-pressed={active}
  title={`${description ? `${description}\n` : ''}${active ? 'Restore normal visibility' : 'Make overlay more visible'}`}
  onpointerdown={event => event.stopPropagation()}
  onclick={event => {
    event.stopPropagation()
    onclick()
  }}>{label}</button
>

<style lang="postcss">
  @reference "../../../app.css";
  .chart-overlay-label {
    @apply pointer-events-auto cursor-pointer rounded-sm border-0 bg-transparent p-0 text-left;
    font: inherit;
    color: inherit;
  }
  .chart-overlay-label[aria-pressed='true'] {
    @apply font-semibold text-base-content;
  }
</style>

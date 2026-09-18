<script lang="ts">
  type Props = { label: string }
  let { label }: Props = $props()
</script>

<span
  class="conflict-marker tooltip tooltip-top"
  data-tip={label}
  aria-label={label}
  role="img"
>
  <span class="conflict-marker__glyph" aria-hidden="true">{'\u26A0'}</span>
</span>

<style lang="postcss">
  @reference "../../app.css";

  .conflict-marker {
    @apply inline-flex size-3.5 flex-none items-center justify-center text-warning leading-none;
    font-size: 0.875rem;
  }
  .conflict-marker__glyph {
    @apply inline-block translate-y-px;
  }
</style>

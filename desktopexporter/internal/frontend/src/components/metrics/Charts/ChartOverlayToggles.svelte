<script lang="ts">
  /*
   * ChartOverlayToggles: generic pill-row for horizontal reference-line
   * overlays on a chart. Pure presentation -- no context coupling, no
   * knowledge of what the overlays actually annotate. Parent passes in
   * the available overlays, the active set, and a per-overlay label
   * snippet (so the dual-number "Σ 4,231 / 12,847" labels for Sum can
   * coexist with simpler labels histograms will use later).
   *
   * Each pill is a single button with aria-pressed = active/inactive,
   * styled to feel like a binary toggle (DaisyUI badge-soft semantics
   * mapped through drawer-header-btn). Multi-select: toggling one pill
   * does not affect others.
   */
  import type { Snippet } from 'svelte'

  type OverlayDefinition<TId extends string> = {
    id: TId
    /** Default-rendered text label when no `label` snippet is provided
     *  for this overlay; useful for histograms that just want "p95". */
    fallbackLabel: string
  }

  type Props<TId extends string> = {
    overlays: OverlayDefinition<TId>[]
    activeOverlays: ReadonlySet<TId>
    onToggle: (id: TId) => void
    /** Optional per-id label snippet. Receives the overlay id as its
     *  one arg so a single shared snippet can render different text
     *  per pill if needed; receives nothing else (the parent already
     *  closed over the value source). */
    label?: Snippet<[id: TId]>
    /** Accessible label for the group; defaults to "Chart overlays". */
    ariaLabel?: string
  }

  // Svelte 5 + generics: type the props through a generic component
  // signature. The TypeScript wrapper around the script runs fine here
  // because we only use TId positionally; the language server infers
  // it from the `overlays` array shape at the call site.
  let {
    overlays,
    activeOverlays,
    onToggle,
    label,
    ariaLabel = 'Chart overlays',
  }: Props<string> = $props()
</script>

<div
  class="chart-overlay-toggles"
  role="group"
  aria-label={ariaLabel}
>
  {#each overlays as overlay (overlay.id)}
    {@const active = activeOverlays.has(overlay.id)}
    <button
      type="button"
      class="chart-overlay-toggles__pill drawer-header-btn {active
        ? 'chart-overlay-toggles__pill--active'
        : 'drawer-header-btn--inactive'}"
      aria-pressed={active}
      onclick={() => onToggle(overlay.id)}
    >
      {#if label}
        {@render label(overlay.id)}
      {:else}
        {overlay.fallbackLabel}
      {/if}
    </button>
  {/each}
</div>

<style lang="postcss">
  @reference "../../../app.css";

  .chart-overlay-toggles {
    @apply flex min-w-0 items-center gap-1;
  }

  .chart-overlay-toggles__pill {
    @apply px-2 text-xs;
  }

  /* Active variant: tint the pill so the toggled-on state is obvious
     against the surrounding header chrome. Reuses primary tone so the
     accent matches the rest of the app's "this is selected" cue. */
  .chart-overlay-toggles__pill--active {
    background-color: color-mix(in oklab, var(--color-primary) 18%, transparent);
    color: var(--color-base-content);
    @apply font-medium;
  }
</style>

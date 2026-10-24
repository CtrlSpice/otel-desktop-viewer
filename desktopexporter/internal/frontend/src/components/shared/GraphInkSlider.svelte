<script lang="ts">
  import {
    graphInkCurve,
    graphSwatch,
    availableCurveShade,
    GRAPH_CURVE_STEPS,
  } from '@/themes/graph-inks'

  let {
    ink,
    label,
    value,
    unavailable,
    onchange,
    close,
  }: {
    ink: string
    label: string
    value?: string
    unavailable: ReadonlySet<string>
    onchange: (id: string) => void
    close: () => void
  } = $props()

  const curve = $derived(graphInkCurve(ink))
  const selected = $derived(graphSwatch(value))
  const current = $derived(selected?.ink === ink ? selected : undefined)
  const shade = $derived(
    availableCurveShade(
      curve,
      current?.position ?? GRAPH_CURVE_STEPS / 2,
      unavailable
    )
  )
  const gradient = $derived(
    `linear-gradient(to right, ${curve
      .filter((_, index) => index % 25 === 0)
      .map(swatch => swatch.color)
      .join(', ')})`
  )

  function move(input: HTMLInputElement) {
    const position = Number(input.value)
    const next = availableCurveShade(
      curve,
      position,
      unavailable,
      position < (shade?.position ?? 0) ? -1 : 1
    )
    if (next) {
      input.value = String(next.position)
      onchange(next.id)
    }
  }
</script>

<div class="ink-slider">
  <input
    type="range"
    min={0}
    max={GRAPH_CURVE_STEPS}
    step={1}
    value={shade?.position ?? 0}
    disabled={!shade}
    data-ink-slider={ink}
    data-selected={Boolean(current)}
    aria-label={`${label}, shade`}
    aria-valuetext={shade ? `${label}, ${shade.color}` : 'All shades assigned'}
    style:--ink-gradient={gradient}
    style:--ink-color={shade?.color}
    oninput={event => move(event.currentTarget)}
  />
  <button
    type="button"
    class="item-state item-state--menu flex size-8 shrink-0 cursor-pointer items-center justify-center rounded-full"
    aria-label={`Use ${label}, ${shade?.color ?? 'unavailable'}`}
    aria-pressed={Boolean(current)}
    title={shade?.color}
    disabled={!shade}
    onclick={() => {
      if (shade) {
        onchange(current?.id ?? shade.id)
        close()
      }
    }}
  >
    <span
      class="size-2 rounded-full"
      style:background-color={shade?.color}
      aria-hidden="true"
    ></span>
  </button>
</div>

<style lang="postcss">
  @reference '../../app.css';
  .ink-slider {
    @apply flex h-8 w-56 max-w-full items-center gap-2;
  }
  input {
    appearance: none;
    min-width: 0;
    width: 100%;
    height: 2rem;
    background: transparent;
    cursor: pointer;
    border-radius: var(--radius-field);
  }
  input::-webkit-slider-runnable-track {
    height: 0.5rem;
    border-radius: 999px;
    background: var(--ink-gradient);
  }
  input::-moz-range-track {
    height: 0.5rem;
    border-radius: 999px;
    background: var(--ink-gradient);
  }
  input::-webkit-slider-thumb {
    appearance: none;
    width: 1rem;
    height: 1rem;
    margin-top: -0.25rem;
    border: 2px solid var(--color-base-content);
    border-radius: 50%;
    background: var(--ink-color);
  }
  input::-moz-range-thumb {
    box-sizing: border-box;
    width: 1rem;
    height: 1rem;
    border: 2px solid var(--color-base-content);
    border-radius: 50%;
    background: var(--ink-color);
  }
  input:focus-visible {
    outline: 2px solid var(--color-primary);
    outline-offset: 2px;
  }
  input:disabled {
    cursor: not-allowed;
    opacity: 0.25;
  }
</style>

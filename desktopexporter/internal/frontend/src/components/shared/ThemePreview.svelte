<script lang="ts">
  import JsonEditor from './JsonEditor.svelte'
  import SignalCard from './SignalCard.svelte'
  import SignalBadges from './SignalBadges.svelte'
  import SignalDate from './SignalDate.svelte'
  import BubbleBreakdown from './BubbleBreakdown.svelte'
  import { GRAPH_SWATCHES } from '@/themes/graph-inks'
  import { serviceInkSlots } from '@/themes/service-ink-slots'
  import type { AccentInkId } from '@/themes/accent-inks'

  let { ink }: { ink: AccentInkId } = $props()
  // Fixed sample slots share the real ranking without registering service owners.
  const slots = $derived(serviceInkSlots(ink))
  const services = $derived([
    {
      key: 'checkout-api',
      label: 'checkout-api',
      count: 6,
      color: GRAPH_SWATCHES[slots[0]!]!.color,
    },
    {
      key: 'postgres',
      label: 'postgres',
      count: 3,
      color: GRAPH_SWATCHES[slots[1]!]!.color,
    },
    {
      key: 'cache',
      label: 'cache',
      count: 2,
      color: GRAPH_SWATCHES[slots[2]!]!.color,
    },
    {
      key: 'payments',
      label: 'payments',
      count: 1,
      color: GRAPH_SWATCHES[slots[3]!]!.color,
    },
  ])
  let selected = $state(true)
</script>

<section class="theme-preview" aria-label="Theme preview (sample trace)">
  <SignalCard
    id="theme-preview-trace"
    title="GET /checkout"
    {selected}
    onclick={() => (selected = !selected)}
  >
    {#snippet badge()}<SignalBadges
        signal="trace"
        spanCount={12}
        errorCount={1}
      />{/snippet}
    {#snippet subtitleContent()}
      <BubbleBreakdown
        parts={services}
        defaultKey="checkout-api"
        label="Spans by service"
        barLabel="Sample service breakdown by span count"
        unit="span"
        tooltipOnly
      />
    {/snippet}
    {#snippet dateEnd()}<span class="shrink-0"
        >Duration: <span class="text-base-content">42 ms</span></span
      >{/snippet}
    {#snippet date()}<SignalDate
        label="Start"
        timestamp={1770000000000000000n}
      />{/snippet}
  </SignalCard>
  <div class="theme-preview__code">
    <h3 class="mb-2 text-sm font-normal text-subtle">JSON</h3>
    <JsonEditor
      label="Theme preview JSON (sample trace)"
      text={'{\n  "name": "GET /checkout",\n  "duration_ns": 42000000,\n  "attributes": {\n    "http.request.method": "GET",\n    "url.path": "/checkout",\n    "http.response.status_code": 200\n  }\n}'}
      raw
    />
  </div>
</section>

<style lang="postcss">
  @reference '../../app.css';

  .theme-preview {
    @apply flex min-w-0 flex-col gap-3 overflow-hidden rounded-xl bg-base-100 text-base-content;
  }

  .theme-preview__code {
    @apply min-w-0 overflow-hidden p-3;
  }
</style>

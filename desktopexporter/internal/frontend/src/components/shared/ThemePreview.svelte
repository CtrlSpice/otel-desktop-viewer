<script lang="ts">
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { BarChartHorizontalIcon } from '@hugeicons/core-free-icons'
  import JsonEditor from './JsonEditor.svelte'
  import SignalCard from './SignalCard.svelte'
  import SignalBadges from './SignalBadges.svelte'
  import SignalDate from './SignalDate.svelte'
  import BubbleBreakdown from './BubbleBreakdown.svelte'
  import DetailProperty from './DetailProperty.svelte'
  import { GRAPH_INKS } from '@/themes/graph-inks'

  // Fixed sample colours must not allocate service identities in saved palettes.
  const services = [
    {
      key: 'checkout-api',
      label: 'checkout-api',
      count: 9,
      color: GRAPH_INKS.find(ink => ink.id === 'kon-peki')!.swatches[3]!.color,
    },
    {
      key: 'postgres',
      label: 'postgres',
      count: 3,
      color: GRAPH_INKS.find(ink => ink.id === 'murasaki-shikibu')!.swatches[3]!
        .color,
    },
  ]
  let selected = $state(true)
</script>

<section class="theme-preview" aria-label="Theme preview (sample trace)">
  <div class="theme-preview__chrome">
    <HugeiconsIcon
      icon={BarChartHorizontalIcon}
      size={16}
      strokeWidth={1.5}
      class="theme-preview__accent text-primary"
      aria-hidden="true"
    />
    <span>Trace preview</span>
  </div>
  <div class="theme-preview__content">
    <div class="flex min-w-0 flex-col gap-3">
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
        {#snippet meta()}<span>Duration: 42 ms</span>{/snippet}
        {#snippet date()}<SignalDate
            label="Started"
            timestamp={1770000000000000000n}
          />{/snippet}
      </SignalCard>
      <div>
        <h3 class="mb-1 px-3 text-sm font-normal text-subtle">Attributes</h3>
        <table class="w-full table-fixed" aria-label="Sample span attributes">
          <tbody>
            <DetailProperty
              fieldName="service.name"
              fieldType="string"
              fieldValue="checkout-api"
              origin="Resource"
            />
            <DetailProperty
              fieldName="http.request.method"
              fieldType="string"
              fieldValue="GET"
            />
            <DetailProperty
              fieldName="http.response.status_code"
              fieldType="int64"
              fieldValue="200"
            />
          </tbody>
        </table>
      </div>
    </div>
    <div class="theme-preview__code">
      <h3 class="mb-2 text-sm font-normal text-subtle">JSON</h3>
      <JsonEditor
        label="Theme preview JSON (sample trace)"
        text={'{\n  "name": "GET /checkout",\n  "duration_ns": 42000000,\n  "attributes": {\n    "http.request.method": "GET",\n    "url.path": "/checkout",\n    "http.response.status_code": 200\n  }\n}'}
        raw
      />
    </div>
  </div>
</section>

<style lang="postcss">
  @reference '../../app.css';

  .theme-preview {
    @apply overflow-hidden rounded-xl bg-base-200 text-base-content;
    container-type: inline-size;
  }

  .theme-preview__chrome {
    @apply flex items-center gap-2 bg-base-300 px-3 py-2 text-sm;
  }

  .theme-preview__content {
    @apply grid gap-3 bg-base-100 p-3;
    grid-template-columns: minmax(0, 1fr) minmax(0, 1.2fr);
  }

  .theme-preview__code {
    @apply min-w-0 overflow-hidden p-3;
  }

  @container (max-width: 36rem) {
    .theme-preview__content {
      grid-template-columns: minmax(0, 1fr);
    }
  }
</style>

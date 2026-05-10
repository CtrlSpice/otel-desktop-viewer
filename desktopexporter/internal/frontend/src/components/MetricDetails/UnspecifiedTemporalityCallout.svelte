<script lang="ts">
  // Renders the "this metric stream has aggregationTemporality =
  // Unspecified" condition. The OTel proto enum literally says
  // "MUST not be used", so the spec does the comedic heavy lifting.
  // Subtle Mufasa "you must never go there, Simba" energy on the
  // detail panel; just a quiet text label in the spark slot.
  //
  // Two variants share one component because the visual language
  // ("you sent us something the spec forbids") is the same -- only
  // the size and the meme-vs-no-meme decision differs.
  import memeImage from '@/assets/images/temporalityUnspecified.jpg'

  type Props = {
    size?: 'mini' | 'full'
  }

  let { size = 'full' }: Props = $props()

  // Deep-link to the proto enum line. Pinned to main: this enum has
  // been stable since the metrics proto was finalised and is unlikely
  // to move. If the line drifts, the link still lands on the file --
  // the comment ("MUST not be used") is what's doing the work.
  const SPEC_URL =
    'https://github.com/open-telemetry/opentelemetry-proto/blob/main/opentelemetry/proto/metrics/v1/metrics.proto#L286'
</script>

{#if size === 'mini'}
  <span class="callout-mini" title="aggregationTemporality = Unspecified">
    unspecifiedTemporality
  </span>
{:else}
  <div class="callout-full">
    <img
      src={memeImage}
      alt="Mufasa to Simba: 'You must never go there, Simba.'"
      class="callout-meme"
    />
    <p class="callout-headline">aggregationTemporality is Unspecified.</p>
    <a
      class="callout-link"
      href={SPEC_URL}
      target="_blank"
      rel="noreferrer noopener"
    >
      You must never go there, Simba.
    </a>
  </div>
{/if}

<style lang="postcss">
  @reference "../../app.css";

  .callout-mini {
    @apply inline-flex items-center font-mono italic text-[0.7rem] tracking-tight text-base-content/45;
  }

  .callout-full {
    @apply flex h-full flex-col items-center justify-center gap-3 px-4 py-8 text-center;
  }

  .callout-meme {
    @apply max-h-64 max-w-full rounded-lg object-contain;
  }

  .callout-headline {
    @apply font-mono text-sm text-base-content/70;
  }

  .callout-link {
    @apply text-sm italic text-base-content/55 underline decoration-dotted underline-offset-4;
    @apply hover:text-base-content/80;
  }
</style>

<script lang="ts">
  // Renders the "this metric stream has aggregationTemporality =
  // Unspecified" condition. The OTel proto enum literally says
  // "MUST not be used", so the spec does the comedic heavy lifting.
  // Subtle Mufasa "you must never go there, Simba" energy on the
  // detail panel; just a quiet text label in the spark slot.
  //
  // Two variants share one component because the visual language
  // ("you sent us something the spec forbids") is the same -- only
  // the size and the ascii-vs-no-ascii decision differs.

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
  <!--
    Single ASCII vignette. white-space: pre + monospace + horizontal
    scroll on narrow panes preserves the alignment exactly. The
    headline is gone (the art carries the spec quote); the caption
    underneath echoes the punchline and links to the proto enum line
    so the diagnostic is one click from the source of truth.
  -->
  <div
    class="callout-full"
    role="img"
    aria-label="Lulu the axolotl and her telescope contemplate AGGREGATION_TEMPORALITY_UNSPECIFIED, which the OTel spec says MUST not be used."
  >
    <div>
      <pre class="callout-ascii"
>╭───────────────────────────────────────────╮
│ Look, Lulu. AggregationTemporality
│ defines how a metric aggregator reports
│ aggregated values. It describes how
│ those values relate to the time interval  
│ over which they are aggregated.    ✧°⊹˖                            ✧  °⊹
╰────╮─────────────────────────✧ ˚ ⊹₊`AGGREGATION_TEMPORALITY_CUMULATIVE`
                                      AGGREGATION_TEMPORALITY_DELTA ⊹
≡(   ó‿ò )≡  =(◕‿◕ )=  🔭            ⊹˖°                         ⊹ ⋆.
▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔⟡</pre>
    </div>

    <div>
      <pre class="callout-ascii"
>        ╭────────────────────────────────╮
        │ But what's that shadowy place  
        │ over there?                    
        ╰──────────╭─────────────────────▒▓▓██████▓▒░
  ≡(  ᵕÓᯅÒ )≡  =( •𐃷•)=  🔭       ░▒▓█`AGGREGATION_TEMPORALITY_UNSPECIFIED`
▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔░▒▒▒▓▓████▓▒░</pre>
    </div>

    <div>
      <pre class="callout-ascii"
>╭───────────────────────────────╮
│ UNSPECIFIED is the default    
│ AggregationTemporality, Lulu. 
│                               
│ It MUST not be used."         
╰────────╮──────────────────────╯░░▒▒▒▓▓███████████▓▒░
  ≡(   òДó )≡  =(•ᴖ• )=  🔭       ░▒▓█`AGGREGATION_TEMPORALITY_UNSPECIFIED`
▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔░▒▒▒▓▓████▓▒░</pre>
    </div>

    <a
      class="callout-link"
      href={SPEC_URL}
      target="_blank"
      rel="noreferrer noopener"
    >
      It MUST not be used.
    </a>
  </div>
{/if}

<style lang="postcss">
  @reference "../../../app.css";

  .callout-mini {
    @apply inline-flex items-center font-mono italic text-[0.7rem] tracking-tight text-base-content/45;
  }

  .callout-full {
    @apply flex h-full flex-col items-center justify-center gap-3 px-4 py-8 text-center;
  }

  /*
   * The art is wider than the panel can be at narrow splits, and
   * re-flowing would garble the alignment. Scroll horizontally
   * instead. Tab size 4 matches the source so the panels (which use
   * `\t` runs for indentation) line up. inline-block + max-w-full
   * keeps the box from claiming the entire callout width on wide
   * panels.
   */
  .callout-ascii {
    @apply inline-block max-w-full overflow-x-auto rounded-lg border border-base-300/60 bg-base-200/40 px-4 py-3 text-left font-mono text-[0.7rem] leading-tight text-base-content/75;
    white-space: pre;
    tab-size: 4;
    -moz-tab-size: 4;
  }

  .callout-link {
    @apply text-sm italic text-base-content/55 underline decoration-dotted underline-offset-4;
    @apply hover:text-base-content/80;
  }
</style>

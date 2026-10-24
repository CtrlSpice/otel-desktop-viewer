<script lang="ts">
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { MonitorIcon } from '@hugeicons/core-free-icons'
  import JsonEditor from './JsonEditor.svelte'
</script>

<section class="theme-preview" aria-label="Theme preview (sample telemetry)">
  <div class="theme-preview__chrome">
    <HugeiconsIcon
      icon={MonitorIcon}
      size={16}
      strokeWidth={1.5}
      aria-hidden="true"
    />
    <span>Preview</span>
  </div>
  <div class="theme-preview__content">
    <div class="theme-preview__query text-subtle">Search telemetry…</div>
    <div class="theme-preview__record">
      <div class="flex items-center gap-2">
        <span class="size-2 shrink-0 rounded-full bg-primary" aria-hidden="true"
        ></span>
        <span class="text-lg">GET /checkout</span>
      </div>
      <div class="flex justify-between gap-2 text-subtle">
        <span>checkout</span><span>42 ms</span>
      </div>
    </div>
    <div class="theme-preview__field">
      <span class="text-subtle">service.name</span>
      <span>checkout</span>
    </div>
    <div class="theme-preview__code">
      <JsonEditor
        label="Theme preview JSON (sample telemetry)"
        text={'{\n  "service.name": "checkout",\n  "duration_ms": 42,\n  "ok": true\n}'}
        raw
      />
    </div>
  </div>
</section>

<style lang="postcss">
  @reference '../../app.css';

  .theme-preview {
    @apply overflow-hidden rounded-xl border border-base-300 bg-base-200 text-base-content;
  }

  .theme-preview__chrome {
    @apply flex items-center gap-2 bg-base-300 px-3 py-2 text-sm;
  }

  .theme-preview__content {
    @apply flex flex-col gap-3 bg-base-100 p-3;
  }

  .theme-preview__query {
    @apply rounded-full border px-3 py-1.5 text-sm;
    background-color: var(--color-input);
    border-color: var(--color-input-border);
  }

  .theme-preview__record {
    @apply flex flex-col gap-1 rounded-xl p-3;
    background-color: var(--color-highlight-med);
  }

  .theme-preview__record:hover {
    background-color: var(--color-highlight-low);
  }

  .theme-preview__field {
    @apply flex flex-col gap-1 rounded-xl bg-base-200 p-3;
  }

  .theme-preview__code {
    @apply min-w-0 overflow-hidden rounded-xl border border-base-300 bg-base-100 p-3;
  }
</style>

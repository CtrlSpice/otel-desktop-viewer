<script lang="ts">
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { ArrowRight01Icon, Cancel01Icon } from '@hugeicons/core-free-icons'
  import type { SearchSignal } from '@/search/model'

  let {
    element = $bindable(null),
    signal,
  }: {
    element?: HTMLDialogElement | null
    signal: SearchSignal
  } = $props()

  const titleID = $props.id()
  const examples = $derived(
    signal === 'logs'
      ? [
          { label: 'Filter a field', query: 'body contains "disk"' },
          {
            label: 'Combine filters',
            query: 'severityNumber >= 17 and body contains "disk"',
          },
        ]
      : signal === 'traces'
        ? [
            { label: 'Filter a field', query: 'name contains "http"' },
            {
              label: 'Combine filters',
              query: 'kind = Server and statusCode = Error',
            },
          ]
        : [
            { label: 'Filter a field', query: 'name contains "requests"' },
            {
              label: 'Combine filters',
              query: 'type = Gauge and name contains "requests"',
            },
          ]
  )
  const attributeScope = $derived(
    signal === 'logs' ? 'log' : signal === 'traces' ? 'span' : 'datapoint'
  )
</script>

<dialog
  bind:this={element}
  class="help-dialog popover-surface"
  aria-labelledby={titleID}
  closedby="any"
  tabindex="-1"
>
  <header class="help-dialog__header">
    <h2 id={titleID}>Search {signal}</h2>
    <button
      type="button"
      class="header-icon-button tooltip tooltip-left"
      aria-label="Close search help"
      data-tip="Close search help"
      onclick={() => element?.close()}
    >
      <HugeiconsIcon
        icon={Cancel01Icon}
        size={16}
        strokeWidth={1.5}
        aria-hidden="true"
      />
    </button>
  </header>
  <div class="help-dialog__body">
    <p>
      Search text across fields, attributes and values, or narrow the results
      with a query.
    </p>
    <dl class="help-examples">
      <div>
        <dt>Find text</dt>
        <dd><pre><code>checkout</code></pre></dd>
      </div>
      {#each examples as example (example.label)}
        <div>
          <dt>{example.label}</dt>
          <dd><pre><code>{example.query}</code></pre></dd>
        </div>
      {/each}
    </dl>
    <details class="help-reference">
      <summary>
        <HugeiconsIcon
          icon={ArrowRight01Icon}
          size={16}
          strokeWidth={1.5}
          aria-hidden="true"
        />
        Query reference
      </summary>
      <div class="help-reference__body">
        <ul>
          <li>
            Join conditions with <code>and</code> or <code>or</code>.
            <code>and</code> is evaluated first; use parentheses to group conditions.
          </li>
          <li>Quote values with spaces: <code>"checkout service"</code>.</li>
          <li>
            Mix text search and filters with <code>search("checkout")</code>.
          </li>
          <li>Limit results by adding <code>| limit 100</code>.</li>
          <li>
            Attribute suggestions include scope and received type, such as <code
              >attr({attributeScope}, "attempts", int64)</code
            >. This matches integers without converting other types.
          </li>
        </ul>
        <table>
          <caption class="sr-only">Query operators</caption>
          <tbody>
            <tr
              ><th scope="row">Equal</th><td><code>=</code>, <code>!=</code></td
              ></tr
            >
            <tr
              ><th scope="row">Compare</th><td
                ><code>&gt;</code>, <code>&lt;</code>, <code>&gt;=</code>,
                <code>&lt;=</code></td
              ></tr
            >
            <tr
              ><th scope="row">Text</th><td
                ><code>contains</code>, <code>not contains</code>,
                <code>^</code>
                (starts with), <code>$</code> (ends with)</td
              ></tr
            >
            <tr
              ><th scope="row">Pattern</th><td
                ><code>=~</code>, <code>!~</code>, <code>regexp</code><span
                  >Matches the full value. Use <code>.*checkout.*</code> for a substring.</span
                ></td
              ></tr
            >
            <tr
              ><th scope="row">List</th><td
                ><code>in</code>, <code>not in</code><span
                  >Use square brackets: <code>["checkout", "payments"]</code
                  >.</span
                ></td
              ></tr
            >
            <tr
              ><th scope="row">Missing</th><td
                ><code>= null</code>, <code>!= null</code><span
                  ><code>null</code> or <code>nil</code> checks absence. Quoted
                  <code>"null"</code> matches literal text.</span
                ></td
              ></tr
            >
          </tbody>
        </table>
      </div>
    </details>
  </div>
  <footer class="help-dialog__footer">
    <span><kbd>Enter</kbd> Run query</span>
    <span><kbd>Ctrl + Space</kbd> Suggestions</span>
    <span><kbd>Esc</kbd> Close</span>
  </footer>
</dialog>

<style lang="postcss">
  @reference "../../../app.css";

  .help-dialog {
    position: fixed;
    inset: 0;
    margin: auto;
    padding: 0;
    width: min(48rem, calc(100vw - 2rem));
    max-width: calc(100vw - 2rem);
    height: fit-content;
    max-height: min(44rem, calc(100dvh - 2rem));
    overscroll-behavior: contain;
  }
  .help-dialog[open] {
    @apply flex flex-col;
  }
  .help-dialog::backdrop {
    background-color: rgb(0 0 0 / 20%);
  }
  .help-dialog__header {
    @apply flex shrink-0 items-center justify-between gap-2 px-4 py-2;
  }
  .help-dialog__header h2 {
    @apply min-w-0 text-lg font-normal;
  }
  .help-dialog__body {
    @apply flex min-h-0 flex-col gap-4 overflow-y-auto px-4 py-3;
    overscroll-behavior: contain;
  }
  .help-examples {
    @apply flex flex-col gap-2;
  }
  .help-examples > div {
    @apply grid items-center gap-2;
    grid-template-columns: 8rem minmax(0, 1fr);
  }
  .help-examples dt {
    @apply text-subtle;
  }
  .help-examples dd {
    @apply min-w-0;
  }
  pre {
    @apply rounded-lg bg-base-100 px-3 py-2;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
  code {
    @apply font-mono text-sm text-base-content;
  }
  .help-reference summary {
    @apply flex cursor-pointer list-none items-center gap-2 rounded-lg py-2 text-subtle;
  }
  .help-reference summary::-webkit-details-marker {
    display: none;
  }
  .help-reference summary:hover {
    background: var(--item-hover-bg);
  }
  .help-reference[open] summary :global(svg) {
    rotate: 90deg;
  }
  .help-reference__body {
    @apply flex flex-col gap-4 pt-3;
  }
  .help-reference ul {
    @apply space-y-2;
  }
  .help-reference table {
    @apply w-full border-collapse text-left;
  }
  .help-reference th {
    @apply w-32 py-2 pr-3 align-top font-normal text-subtle;
  }
  .help-reference td {
    @apply py-2 align-top;
    overflow-wrap: anywhere;
  }
  .help-reference td span {
    @apply mt-1 block text-subtle;
  }
  .help-dialog__footer {
    @apply flex shrink-0 flex-wrap gap-x-6 gap-y-2 px-4 py-3 text-subtle;
  }
  .help-dialog__footer span {
    @apply flex items-center gap-2;
  }
  kbd {
    @apply font-sans text-sm text-base-content;
  }
  @media (max-width: 36rem) {
    .help-examples > div {
      grid-template-columns: minmax(0, 1fr);
    }
  }
</style>

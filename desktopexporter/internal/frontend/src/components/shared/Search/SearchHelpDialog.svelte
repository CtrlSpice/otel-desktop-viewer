<script lang="ts">
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { Cancel01Icon } from '@hugeicons/core-free-icons'
  import type { SearchSignal } from '@/search/model'
  import type { SearchDefinition } from './search-editor-api'

  let {
    element = $bindable(null),
    signal,
    definition,
  }: {
    element?: HTMLDialogElement | null
    signal?: SearchSignal
    definition?: SearchDefinition
  } = $props()

  const titleID = $props.id()
  const examples = $derived(
    definition?.examples ??
      (signal === 'logs'
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
            ])
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
    <h2 id={titleID}>Search {definition?.label ?? signal}</h2>
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
    <section class="help-reference" aria-label="Query reference">
      <table>
        <caption>Query reference</caption>
        <tbody>
          <tr
            ><th scope="row">Find text</th><td
              ><code>checkout</code><span
                >Case-insensitive tokens, not substrings. Full IDs, URLs and
                dotted names, plus punctuation-separated words, are searchable.
                Results keep their existing order.</span
              ></td
            ></tr
          >
          {#if !definition}
            <tr
              ><th scope="row">Attribute</th><td
                >Type its name and select a suggestion.<span
                  >Scope and type appear beside the name. You can also write
                  <code>attributes['key']</code> for any owner, or
                  <code>resource.attributes['key']::VARCHAR</code> for an exact owner
                  and received type.</span
                ></td
              ></tr
            >
          {/if}
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
              ><code>CONTAINS</code>, <code>NOT CONTAINS</code>,
              <code>LIKE</code>, <code>NOT LIKE</code><span
                >For a selected field or attribute, contains still matches
                substrings. LIKE uses <code>%</code> for any text and
                <code>_</code> for one character.</span
              ></td
            ></tr
          >
          <tr
            ><th scope="row">Pattern</th><td
              ><code>REGEXP</code>, <code>NOT REGEXP</code><span
                >Matches the full value. Use <code>.*checkout.*</code> for a substring.</span
              ></td
            ></tr
          >
          <tr
            ><th scope="row">List</th><td
              ><code>in</code>, <code>not in</code><span
                >Use parentheses: <code>('checkout', 'payments')</code>.</span
              ></td
            ></tr
          >
          <tr
            ><th scope="row">Missing</th><td
              ><code>IS NULL</code>, <code>IS NOT NULL</code><span
                >Checks absence. Quoted <code>'null'</code> matches literal text.</span
              ></td
            ></tr
          >
          <tr
            ><th scope="row">Combine</th><td
              ><code>AND</code>, <code>OR</code><span
                ><code>AND</code> is evaluated first. An initial
                <code>WHERE</code>
                is optional.</span
              ></td
            ></tr
          >
          <tr
            ><th scope="row">Group</th><td
              ><code>( … )</code><span
                >Parentheses set the evaluation order.</span
              ></td
            ></tr
          >
          <tr
            ><th scope="row">Quotes</th><td
              ><code>"checkout service"</code><span
                >Quote values containing spaces. Free-text quotes match all
                words, not an exact phrase.</span
              ></td
            ></tr
          >
          <tr
            ><th scope="row">Text with filters</th><td
              ><code>search("checkout")</code><span
                >Combine text search with a condition using <code>and</code> or
                <code>or</code>. Text terms must match one record and its parent
                context, not different events, datapoints or results.</span
              ></td
            ></tr
          >
          <tr
            ><th scope="row">Limit</th><td
              ><code>LIMIT 100</code><span>Add at the end of the query.</span
              ></td
            ></tr
          >
        </tbody>
      </table>
    </section>
    <section aria-label="Examples">
      <h3>Examples</h3>
      <dl class="help-examples">
        {#each examples as example (example.label)}
          <div>
            <dt>{example.label}</dt>
            <dd><pre><code>{example.query}</code></pre></dd>
          </div>
        {/each}
      </dl>
    </section>
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
  caption,
  h3 {
    @apply pb-2 text-left font-normal text-subtle;
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

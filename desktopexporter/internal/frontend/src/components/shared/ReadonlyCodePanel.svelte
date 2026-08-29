<script module lang="ts">
  import { EditorView } from '@codemirror/view'
  import { HighlightStyle, syntaxHighlighting } from '@codemirror/language'
  import { tags as t } from '@lezer/highlight'
  import { editorColors as c } from '@/components/shared/utils/editor-colors'

  // CodeMirror theme for the readonly code surface. Lives in <script
  // module> so it's evaluated once per module load, not per component
  // instance, and so the heavy CodeMirror imports stay tree-shaken
  // when ReadonlyCodePanel isn't mounted.
  const shellColors = {
    subtle: `var(--readonly-code-subtle, ${c.subtle})`,
    gold: `var(--readonly-code-gold, ${c.gold})`,
    rose: `var(--readonly-code-rose, ${c.rose})`,
    foam: `var(--readonly-code-foam, ${c.foam})`,
    iris: `var(--readonly-code-iris, ${c.iris})`,
  }

  const shellHighlightStyle = HighlightStyle.define([
    { tag: t.comment, color: shellColors.subtle, fontStyle: 'italic' },
    { tag: t.string, color: shellColors.gold },
    { tag: t.number, color: shellColors.rose },
    { tag: t.variableName, color: shellColors.foam },
    { tag: t.keyword, color: shellColors.iris },
    { tag: t.operator, color: shellColors.subtle },
    { tag: t.meta, color: shellColors.subtle },
    { tag: t.name, color: c.text },
  ])

  const readonlyCodeEditorTheme = EditorView.theme({
    '&': {
      fontSize: '13px',
      fontFamily: '"Atkinson Hyperlegible Mono", ui-monospace, monospace',
      backgroundColor: c.base,
      color: c.text,
    },
    '&.cm-focused': {
      outline: 'none',
    },
    '.cm-scroller': {
      overflow: 'auto',
      fontFamily: 'inherit',
    },
    '.cm-content': {
      padding: '0.75rem 1rem 1rem',
      caretColor: 'transparent',
    },
    '.cm-cursor, .cm-dropCursor': {
      display: 'none',
    },
    '.cm-selectionBackground, &.cm-focused .cm-selectionBackground': {
      backgroundColor: `${c.overlay} !important`,
    },
    '.cm-activeLine': {
      backgroundColor:
        'color-mix(in oklab, var(--color-base-300) 35%, transparent)',
    },
    '.cm-gutters': {
      display: 'none',
    },
  })

  const readonlyCodeTheme = [
    readonlyCodeEditorTheme,
    syntaxHighlighting(shellHighlightStyle),
  ]
</script>

<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import { EditorState } from '@codemirror/state'
  import { StreamLanguage } from '@codemirror/language'
  import { shell } from '@codemirror/legacy-modes/mode/shell'
  import { CheckmarkCircleIcon, CopyIcon } from '@/icons'

  type Props = {
    code: string
    /** Accessible name for the readonly CodeMirror textbox. */
    ariaLabel: string
    class?: string
    /** Accessible name for the copy control. */
    copyLabel?: string
    /** Flush inside a parent chrome card (no border/radius on the panel). */
    embedded?: boolean
  }

  let {
    code,
    ariaLabel,
    class: className = '',
    copyLabel = 'Copy to clipboard',
    embedded = false,
  }: Props = $props()

  let mountEl: HTMLDivElement | undefined = $state()
  let editorView: EditorView | null = null
  let copied = $state(false)

  const shellLanguage = StreamLanguage.define(shell)

  onMount(() => {
    if (!mountEl) return

    editorView = new EditorView({
      state: EditorState.create({
        doc: code,
        extensions: [
          shellLanguage,
          EditorState.readOnly.of(true),
          EditorView.editable.of(false),
          EditorView.contentAttributes.of({ 'aria-label': ariaLabel }),
          EditorView.lineWrapping,
          ...readonlyCodeTheme,
        ],
      }),
      parent: mountEl,
    })
  })

  $effect(() => {
    if (!editorView) return
    const current = editorView.state.doc.toString()
    if (current !== code) {
      editorView.dispatch({
        changes: { from: 0, to: editorView.state.doc.length, insert: code },
      })
    }
  })

  onDestroy(() => {
    editorView?.destroy()
    editorView = null
  })

  async function copyToClipboard() {
    try {
      await navigator.clipboard.writeText(code)
      copied = true
      setTimeout(() => {
        copied = false
      }, 2000)
    } catch (err) {
      console.error('Failed to copy text:', err)
    }
  }
</script>

<div
  class="readonly-code-panel {className}"
  class:readonly-code-panel--embedded={embedded}
>
  {#if !embedded}
    <div class="readonly-code-panel__toolbar">
      <button
        type="button"
        class="readonly-code-panel__copy drawer-header-btn tooltip tooltip-left"
        onclick={copyToClipboard}
        data-tip={copied ? 'Copied!' : copyLabel}
        aria-label={copied ? 'Copied' : copyLabel}
      >
        {#if copied}
          <CheckmarkCircleIcon class="h-4 w-4 shrink-0" aria-hidden="true" />
        {:else}
          <CopyIcon class="h-4 w-4 shrink-0" aria-hidden="true" />
        {/if}
      </button>
    </div>
  {/if}
  <div class="readonly-code-panel__editor" bind:this={mountEl}></div>
</div>

<style lang="postcss">
  @reference "../../app.css";

  .readonly-code-panel {
    @apply relative w-full overflow-hidden rounded-xl border border-base-300 bg-base-100;
  }

  /* Rosé Pine Dawn's display accents are too light for 13px code on Base.
     These stay in the same hue families while clearing AA for small text. */
  :global(html[data-theme='rose-pine-dawn']) .readonly-code-panel {
    --readonly-code-subtle: var(--color-base-content);
    --readonly-code-gold: #6d4f00;
    --readonly-code-rose: #8f435b;
    --readonly-code-foam: var(--color-secondary);
    --readonly-code-iris: #6e577f;
  }

  .readonly-code-panel--embedded {
    @apply rounded-none rounded-b-xl border-0;
  }

  .readonly-code-panel__toolbar {
    @apply pointer-events-none absolute right-0 top-0 z-10 flex justify-end p-1.5;
  }

  .readonly-code-panel__copy {
    @apply pointer-events-auto;
  }

  .readonly-code-panel__editor {
    @apply block min-h-0 w-full border-none bg-base-100 px-0 py-0 shadow-none;
    height: auto;
    min-height: 0;
  }

  .readonly-code-panel__editor :global(.cm-editor) {
    @apply bg-base-100;
  }

  .readonly-code-panel__editor :global(.cm-editor.cm-focused) {
    outline: none;
  }
</style>

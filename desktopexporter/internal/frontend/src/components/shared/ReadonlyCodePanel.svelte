<script module lang="ts">
  import { EditorView } from '@codemirror/view'
  import { syntaxHighlighting } from '@codemirror/language'
  import { editorColors as c } from '@/components/shared/utils/editor-colors'
  import { editorHighlightStyle } from '@/components/shared/utils/editor-highlighting'

  // Share the CodeMirror theme across component instances.
  const readonlyCodeEditorTheme = EditorView.theme({
    '&': {
      fontSize: 'var(--text-base)',
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
      lineHeight: 'var(--text-base--line-height)',
    },
    '.cm-content': {
      padding: '0.75rem 1rem 1rem',
      caretColor: 'transparent',
    },
    '.cm-cursor, .cm-dropCursor': {
      display: 'none',
    },
    '.cm-selectionBackground, &.cm-focused .cm-selectionBackground': {
      backgroundColor: `${c.highlightMed} !important`,
    },
    '.cm-activeLine': {
      backgroundColor: c.activeLine,
    },
    '.cm-gutters': {
      display: 'none',
    },
  })

  const readonlyCodeTheme = [
    readonlyCodeEditorTheme,
    syntaxHighlighting(editorHighlightStyle),
  ]
</script>

<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import { EditorState } from '@codemirror/state'
  import { shellLanguage } from './utils/shell-highlighting'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { CheckmarkCircle02Icon, Copy01Icon } from '@hugeicons/core-free-icons'

  type Props = {
    code: string
    /** Accessible name for the readonly CodeMirror textbox. */
    ariaLabel: string
    class?: string
    /** Accessible name for the copy control. */
    copyLabel?: string
    /** Flush inside a parent chrome card (no border/radius on the panel). */
    embedded?: boolean
    /** Editor only: the caller supplies its surrounding layout and Copy action. */
    bare?: boolean
  }

  let {
    code,
    ariaLabel,
    class: className = '',
    copyLabel = 'Copy to clipboard',
    embedded = false,
    bare = false,
  }: Props = $props()

  let mountEl: HTMLDivElement | undefined = $state()
  let editorView: EditorView | null = null
  let copied = $state(false)

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
          ...(bare
            ? [
                EditorView.theme({
                  '&': { backgroundColor: 'transparent' },
                  '.cm-content': { padding: '0' },
                  '.cm-line': { padding: '0' },
                }),
              ]
            : []),
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

{#if bare}
  <div bind:this={mountEl}></div>
{:else}
  <div
    class="readonly-code-panel {className}"
    class:readonly-code-panel--embedded={embedded}
  >
    {#if !embedded}
      <button
        type="button"
        class="readonly-code-panel__copy drawer-header-btn tooltip tooltip-left"
        onclick={copyToClipboard}
        data-tip={copied ? 'Copied!' : copyLabel}
        aria-label={copied ? 'Copied' : copyLabel}
      >
        {#if copied}
          <HugeiconsIcon
            icon={CheckmarkCircle02Icon}
            size="1em"
            strokeWidth={1.5}
            class="h-4 w-4 shrink-0"
            aria-hidden="true"
          />
        {:else}
          <HugeiconsIcon
            icon={Copy01Icon}
            size="1em"
            strokeWidth={1.5}
            class="h-4 w-4 shrink-0"
            aria-hidden="true"
          />
        {/if}
      </button>
    {/if}
    <div class="readonly-code-panel__editor" bind:this={mountEl}></div>
  </div>
{/if}

<style lang="postcss">
  @reference "../../app.css";

  .readonly-code-panel {
    @apply relative w-full overflow-hidden rounded-xl border-0 bg-base-100;
  }

  .readonly-code-panel--embedded {
    @apply rounded-none rounded-b-xl;
  }

  .readonly-code-panel__copy {
    @apply absolute right-1.5 top-1.5 z-10;
  }
</style>

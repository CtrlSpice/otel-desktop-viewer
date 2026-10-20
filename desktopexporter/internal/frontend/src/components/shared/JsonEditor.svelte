<script lang="ts">
  import { EditorState } from '@codemirror/state'
  import { EditorView } from '@codemirror/view'
  import { foldAll, unfoldAll } from '@codemirror/language'
  import { jsonEditorExtensions } from './utils/json-editor'

  let {
    text,
    label = 'JSON value',
    raw = false,
  }: { text: string; label?: string; raw?: boolean } = $props()
  let host = $state<HTMLDivElement>()
  let view: EditorView | undefined

  export function collapseAll() {
    if (view) foldAll(view)
  }

  export function expandAll() {
    if (view) unfoldAll(view)
  }
  $effect(() => {
    if (!host) return
    view = new EditorView({
      parent: host,
      state: EditorState.create({
        doc: text,
        extensions: [
          jsonEditorExtensions,
          EditorView.contentAttributes.of({
            'aria-label': label,
            tabindex: '0',
          }),
        ],
      }),
    })
    const instance = view
    return () => {
      instance.destroy()
      if (view === instance) view = undefined
    }
  })
</script>

<div
  class="json-editor value-syntax"
  class:raw-json-editor={raw}
  bind:this={host}
></div>

<style lang="postcss">
  @reference "../../app.css";
  .json-editor {
    @apply min-w-0 font-mono text-xs;
  }
</style>

<script lang="ts">
  import { onMount } from 'svelte'
  import { Compartment, EditorState } from '@codemirror/state'
  import { EditorView } from '@codemirror/view'
  import {
    foldAll,
    unfoldAll,
    ensureSyntaxTree,
    foldEffect,
  } from '@codemirror/language'
  import { jsonEditorExtensions } from './utils/json-editor'
  import { jsonReadableStrings } from './utils/json-readable-strings'

  let {
    text,
    label = 'JSON value',
    raw = false,
    defaultOpenDepth,
  }: {
    text: string
    label?: string
    raw?: boolean
    defaultOpenDepth?: number
  } = $props()
  let host = $state<HTMLDivElement>()
  let view = $state.raw<EditorView>()
  const presentation = new Compartment()
  let appliedDepth: number | undefined

  function initialFolds(instance: EditorView) {
    appliedDepth = defaultOpenDepth
    if (defaultOpenDepth === undefined) return
    const tree = ensureSyntaxTree(instance.state, instance.state.doc.length, 50)
    if (!tree) return
    const effects: ReturnType<typeof foldEffect.of>[] = []
    let depth = 0
    tree.iterate({
      enter(node) {
        if (node.name !== 'Object' && node.name !== 'Array') return
        const firstArrayItem =
          depth === defaultOpenDepth &&
          node.node.parent?.name === 'Array' &&
          node.node.prevSibling?.name === '['
        if (
          depth >= defaultOpenDepth! &&
          !firstArrayItem &&
          node.to - node.from > 2
        ) {
          effects.push(foldEffect.of({ from: node.from + 1, to: node.to - 1 }))
          return false
        }
        depth++
      },
      leave(node) {
        if (node.name === 'Object' || node.name === 'Array') depth--
      },
    })
    instance.dispatch({ effects })
  }

  function presentationExtensions() {
    return [
      ...(raw ? [] : [jsonReadableStrings]),
      EditorView.contentAttributes.of({ 'aria-label': label, tabindex: '0' }),
    ]
  }

  export function collapseAll() {
    if (view) foldAll(view)
  }

  export function expandAll() {
    if (view) unfoldAll(view)
  }
  onMount(() => {
    if (!host) return
    view = new EditorView({
      parent: host,
      state: EditorState.create({
        doc: text,
        extensions: [
          jsonEditorExtensions,
          presentation.of(presentationExtensions()),
        ],
      }),
    })
    const instance = view
    initialFolds(instance)
    return () => {
      instance.destroy()
      if (view === instance) view = undefined
    }
  })

  $effect(() => {
    if (!view) return
    const changed = view.state.doc.toString() !== text
    if (changed) {
      unfoldAll(view)
      view.dispatch({
        changes: { from: 0, to: view.state.doc.length, insert: text },
      })
    }
    view.dispatch({
      effects: presentation.reconfigure(presentationExtensions()),
    })
    if (changed || appliedDepth !== defaultOpenDepth) initialFolds(view)
  })
</script>

<div class="json-editor" class:raw-json-editor={raw} bind:this={host}></div>

<style lang="postcss">
  @reference "../../app.css";
  .json-editor {
    @apply min-w-0 font-mono text-xs;
  }
</style>

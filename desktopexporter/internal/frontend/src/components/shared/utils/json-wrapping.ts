import { syntaxTree } from '@codemirror/language'
import { countColumn } from '@codemirror/state'
import { Decoration, EditorView, ViewPlugin } from '@codemirror/view'

function stringIndents(view: EditorView) {
  const { state } = view
  const columns = new Map<number, number>()
  const tree = syntaxTree(state)
  for (const { from, to } of view.visibleRanges) {
    tree.iterate({
      from,
      to,
      enter(node) {
        // Object values wrap under the key's first character. Standalone array
        // strings wrap under their own first character, after the opening quote.
        if (node.name !== 'PropertyName' && node.name !== 'String') return
        const line = state.doc.lineAt(node.from)
        if (!columns.has(line.from)) {
          const prefix = state.doc.sliceString(line.from, node.from + 1)
          columns.set(line.from, countColumn(prefix, state.tabSize))
        }
        return false
      },
    })
  }
  return Decoration.set(
    [...columns].map(([from, column]) =>
      Decoration.line({
        attributes: {
          class: 'json-string-line',
          style: `--json-string-indent: ${column}ch`,
        },
      }).range(from)
    ),
    true
  )
}

export const jsonWrapping = [
  EditorView.lineWrapping,
  ViewPlugin.define(
    view => ({
      decorations: stringIndents(view),
      update(update) {
        if (
          update.docChanged ||
          update.viewportChanged ||
          syntaxTree(update.startState) !== syntaxTree(update.state)
        ) {
          this.decorations = stringIndents(update.view)
        }
      },
    }),
    { decorations: plugin => plugin.decorations }
  ),
  EditorView.theme({
    '.cm-line.json-string-line': {
      paddingLeft: 'var(--json-string-indent)',
      textIndent: 'calc(-1 * var(--json-string-indent))',
      wordBreak: 'break-all',
    },
  }),
]

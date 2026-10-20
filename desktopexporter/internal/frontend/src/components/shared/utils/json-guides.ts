import { countColumn } from '@codemirror/state'
import { Decoration, EditorView, ViewPlugin } from '@codemirror/view'

function indentGuides(view: EditorView) {
  const lines = new Map<number, number>()
  for (const { from, to } of view.visibleRanges) {
    for (let pos = view.state.doc.lineAt(from).from; pos <= to;) {
      const line = view.state.doc.lineAt(pos)
      const prefix = line.text.slice(
        0,
        line.text.length - line.text.trimStart().length
      )
      if (prefix) lines.set(line.from, countColumn(prefix, view.state.tabSize))
      pos = line.to + 1
    }
  }
  return Decoration.set(
    [...lines].map(([from, columns]) =>
      Decoration.line({
        attributes: {
          class: 'json-indent-guide',
          style: `--json-guide-width: ${columns}ch`,
        },
      }).range(from)
    ),
    true
  )
}

export const jsonGuides = [
  ViewPlugin.define(
    view => ({
      decorations: indentGuides(view),
      update(update) {
        if (update.docChanged || update.viewportChanged) {
          this.decorations = indentGuides(update.view)
        }
      },
    }),
    { decorations: plugin => plugin.decorations }
  ),
  EditorView.theme({
    '.cm-line.json-indent-guide': { position: 'relative' },
    '.cm-line.json-indent-guide::before': {
      content: '""',
      position: 'absolute',
      top: '0',
      bottom: '0',
      left: '0',
      width: 'var(--json-guide-width)',
      pointerEvents: 'none',
      backgroundImage:
        'radial-gradient(circle at 0.5ch 0.5lh, var(--guide-dot-color) var(--guide-dot-radius), transparent var(--guide-dot-radius))',
      backgroundSize: '2ch 1lh',
      backgroundPosition: '0 0.5lh',
      opacity: 'var(--guide-dot-opacity)',
    },
  }),
]

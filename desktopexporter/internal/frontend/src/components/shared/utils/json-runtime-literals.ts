import { Decoration, EditorView, ViewPlugin } from '@codemirror/view'
import { compactValueTokens, isRuntimeLiteral } from '../detail-value'
import { editorColors } from './editor-colors'

/** The JSON grammar supplies folding and ordinary syntax. Use the same lexical
 * rules as compact previews for non-JSON runtime literals in formatted Details. */
function literals(view: EditorView) {
  const ranges: ReturnType<Decoration['range']>[] = []
  const seen = new Set<number>()
  for (const { from, to } of view.visibleRanges) {
    const first = view.state.doc.lineAt(from).number
    const last = view.state.doc.lineAt(to).number
    for (let number = first; number <= last; number++) {
      if (seen.has(number)) continue
      seen.add(number)
      const line = view.state.doc.line(number)
      let offset = line.from
      for (const token of compactValueTokens(line.text)) {
        if (token.kind === 'literal' && isRuntimeLiteral(token.text))
          ranges.push(
            Decoration.mark({ class: 'json-runtime-literal' }).range(
              offset,
              offset + token.text.length
            )
          )
        offset += token.text.length
      }
    }
  }
  return Decoration.set(ranges, true)
}

export const jsonRuntimeLiterals = [
  ViewPlugin.define(
    view => ({
      decorations: literals(view),
      update(update) {
        if (update.docChanged || update.viewportChanged)
          this.decorations = literals(update.view)
      },
    }),
    { decorations: plugin => plugin.decorations }
  ),
  EditorView.theme({
    '.json-runtime-literal, .json-runtime-literal *': {
      color: editorColors.literal,
    },
  }),
]

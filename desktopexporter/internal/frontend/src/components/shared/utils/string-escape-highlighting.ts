import { syntaxTree } from '@codemirror/language'
import { Decoration, EditorView, ViewPlugin } from '@codemirror/view'
import { editorColors } from './editor-colors'
import { jsonEscapeRanges } from './json-escapes'
import { readableJsonStrings } from './json-readable-strings'

function escapes(view: EditorView) {
  const ranges: ReturnType<Decoration['range']>[] = []
  const seen = new Set<number>()
  const decoded = view.state.facet(readableJsonStrings)
  for (const { from, to } of view.visibleRanges) {
    syntaxTree(view.state).iterate({
      from,
      to,
      enter(node) {
        if (
          (node.name !== 'PropertyName' &&
            node.name !== 'QuotedString' &&
            (node.name !== 'String' || decoded)) ||
          seen.has(node.from)
        )
          return
        seen.add(node.from)
        const text = view.state.doc.sliceString(node.from, node.to)
        const matches =
          node.name === 'QuotedString'
            ? Array.from(
                text.matchAll(/\\(?:u[\da-fA-F]{4}|[^\r\n])/g),
                match => ({
                  from: match.index,
                  to: match.index + match[0].length,
                })
              )
            : jsonEscapeRanges(text)
        for (const range of matches) {
          ranges.push(
            Decoration.mark({ class: 'json-escape' }).range(
              node.from + range.from,
              node.from + range.to
            )
          )
        }
        return false
      },
    })
  }
  return Decoration.set(ranges, true)
}

/** JSON and query grammars tag whole strings; colour their escapes without
 * replacing the document or changing the tree used by folding and Copy. */
export const stringEscapeHighlighting = [
  ViewPlugin.define(
    view => ({
      decorations: escapes(view),
      update(update) {
        if (
          update.docChanged ||
          update.viewportChanged ||
          syntaxTree(update.startState) !== syntaxTree(update.state) ||
          update.startState.facet(readableJsonStrings) !==
            update.state.facet(readableJsonStrings)
        )
          this.decorations = escapes(update.view)
      },
    }),
    { decorations: plugin => plugin.decorations }
  ),
  EditorView.theme({
    '.json-escape, .json-escape *': { color: editorColors.escape },
  }),
]

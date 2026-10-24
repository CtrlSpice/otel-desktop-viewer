import { syntaxTree } from '@codemirror/language'
import { Decoration, EditorView, ViewPlugin } from '@codemirror/view'
import { editorColors } from './editor-colors'
import { jsonEscapeRanges, stringQuoteRanges } from './json-escapes'
import { readableJsonStrings } from './json-readable-strings'

function stringMarks(view: EditorView) {
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
            node.name !== 'String') ||
          seen.has(node.from)
        )
          return
        seen.add(node.from)
        const text = view.state.doc.sliceString(node.from, node.to)
        // Properties hides value delimiters and decodes escapes. Empty strings
        // retain their quotes so the value remains visible.
        if (node.name === 'String' && decoded && text !== '""') return false
        for (const range of stringQuoteRanges(text)) {
          ranges.push(
            Decoration.mark({ class: 'json-quote' }).range(
              node.from + range.from,
              node.from + range.to
            )
          )
        }
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

/** JSON and query grammars tag whole strings; colour quotes and escapes without
 * replacing the document or changing the tree used by folding and Copy. */
export const stringSyntaxHighlighting = [
  ViewPlugin.define(
    view => ({
      decorations: stringMarks(view),
      update(update) {
        if (
          update.docChanged ||
          update.viewportChanged ||
          syntaxTree(update.startState) !== syntaxTree(update.state) ||
          update.startState.facet(readableJsonStrings) !==
            update.state.facet(readableJsonStrings)
        )
          this.decorations = stringMarks(update.view)
      },
    }),
    { decorations: plugin => plugin.decorations }
  ),
  EditorView.theme({
    '.json-quote, .json-quote *': { color: editorColors.quote },
    '.json-escape, .json-escape *': { color: editorColors.escape },
  }),
]

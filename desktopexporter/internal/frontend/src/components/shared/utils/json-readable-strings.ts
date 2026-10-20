import { syntaxTree } from '@codemirror/language'
import { Facet } from '@codemirror/state'
import {
  Decoration,
  EditorView,
  ViewPlugin,
  WidgetType,
} from '@codemirror/view'

export const readableJsonStrings = Facet.define<boolean, boolean>({
  combine: values => values.some(Boolean),
})

class DecodedEscape extends WidgetType {
  constructor(readonly text: string) {
    super()
  }
  eq(other: DecodedEscape) {
    return this.text === other.text
  }
  get lineBreaks() {
    return this.text.match(/\r\n?|\n/g)?.length ?? 0
  }
  toDOM() {
    const span = document.createElement('span')
    span.className = 'json-decoded-escape'
    span.textContent = this.text
    return span
  }
}

function strings(view: EditorView) {
  const ranges: ReturnType<Decoration['range']>[] = []
  const seen = new Set<number>()
  for (const { from, to } of view.visibleRanges) {
    syntaxTree(view.state).iterate({
      from,
      to,
      enter(node) {
        // PropertyName is deliberately excluded: keys retain JSON syntax.
        if (node.name !== 'String' || seen.has(node.from)) return
        seen.add(node.from)
        const source = view.state.doc.sliceString(node.from, node.to)
        try {
          JSON.parse(source)
        } catch {
          return false
        }
        // Keep an empty value visible rather than making it look absent.
        if (source === '""') return false
        ranges.push(
          Decoration.replace({}).range(node.from, node.from + 1),
          Decoration.replace({}).range(node.to - 1, node.to)
        )
        // Decode exactly one JSON layer. In particular, a received backslash-n
        // remains backslash-n; it must not become a newline on a second pass.
        for (const match of source.matchAll(
          /(?:\\(?:u[\da-fA-F]{4}|["\\/bfnrt]))+/g
        )) {
          const text = JSON.parse(`"${match[0]}"`) as string
          ranges.push(
            Decoration.replace({ widget: new DecodedEscape(text) }).range(
              node.from + match.index,
              node.from + match.index + match[0].length
            )
          )
        }
        return false
      },
    })
  }
  return Decoration.set(ranges, true)
}

/** A readable presentation over the intact JSON document and its syntax tree. */
export const jsonReadableStrings = [
  readableJsonStrings.of(true),
  ViewPlugin.define(
    view => ({
      decorations: strings(view),
      update(update) {
        if (
          update.docChanged ||
          update.viewportChanged ||
          syntaxTree(update.startState) !== syntaxTree(update.state)
        ) {
          this.decorations = strings(update.view)
        }
      },
    }),
    { decorations: plugin => plugin.decorations }
  ),
  EditorView.theme({
    '.json-decoded-escape': {
      color: 'var(--color-base-content)',
      whiteSpace: 'pre-wrap',
    },
  }),
]

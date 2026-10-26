import { startCompletion } from '@codemirror/autocomplete'
import { StateField, type EditorState } from '@codemirror/state'
import {
  Decoration,
  EditorView,
  WidgetType,
  type DecorationSet,
} from '@codemirror/view'
import type { SearchSignal } from '@/search/model'
import {
  attributeMetadata,
  parseAttributeFieldReference,
  type AttributeField,
} from '../attribute-field-reference'
import { parser } from './query.parser'

class AttributeToken extends WidgetType {
  constructor(
    readonly field: AttributeField,
    readonly length: number
  ) {
    super()
  }

  eq(other: AttributeToken) {
    return (
      this.field.name === other.field.name &&
      this.field.attributeScope === other.field.attributeScope &&
      this.field.type === other.field.type &&
      this.length === other.length
    )
  }

  toDOM(view: EditorView): HTMLElement {
    const button = document.createElement('button')
    button.type = 'button'
    button.className = 'cm-attribute-token'
    button.setAttribute(
      'aria-label',
      `Change attribute ${this.field.name} · ${attributeMetadata(this.field)}`
    )
    button.title = `${this.field.name} · ${attributeMetadata(this.field)}`
    const name = document.createElement('span')
    name.textContent = this.field.name
    const metadata = document.createElement('span')
    metadata.className = 'cm-attribute-token__metadata'
    metadata.textContent = ` · ${attributeMetadata(this.field)}`
    button.append(name, metadata)
    button.addEventListener('mousedown', event => event.preventDefault())
    button.addEventListener('click', () => {
      const from = view.posAtDOM(button)
      view.dispatch({ selection: { anchor: from, head: from + this.length } })
      view.focus()
      startCompletion(view)
    })
    return button
  }

  ignoreEvent() {
    return true
  }
}

export function attributeTokens(signal: SearchSignal) {
  function decorate(state: EditorState): DecorationSet {
    const ranges: ReturnType<Decoration['range']>[] = []
    const source = state.doc.toString()
    parser.parse(source).iterate({
      enter(node) {
        if (node.name !== 'AttributeReference') return
        const field = parseAttributeFieldReference(
          source.slice(node.from, node.to),
          signal
        )
        if (field)
          ranges.push(
            Decoration.replace({
              widget: new AttributeToken(field, node.to - node.from),
            }).range(node.from, node.to)
          )
      },
    })
    return Decoration.set(ranges, true)
  }
  const tokens = StateField.define<DecorationSet>({
    create: decorate,
    update: (value, transaction) =>
      transaction.docChanged ? decorate(transaction.state) : value,
    provide: field => [
      EditorView.decorations.from(field),
      EditorView.atomicRanges.of(view => view.state.field(field)),
    ],
  })
  return tokens
}

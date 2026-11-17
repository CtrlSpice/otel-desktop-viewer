import { isolateHistory } from '@codemirror/commands'
import { EditorSelection } from '@codemirror/state'
import { EditorView } from '@codemirror/view'
import { parser } from './query.parser'

/** Keep query formatting on one line without changing quoted values. */
export function normalizeQueryPaste(
  text: string,
  before = '',
  after = ''
): string {
  if (!/[\r\n]/.test(text)) return text
  let structured = false
  parser.parse(before + text + after).iterate({
    enter(node) {
      if (node.name === 'Comparison' || node.name === 'GlobalText')
        structured = true
    },
  })
  const lineBreaks = /[ \t]*[\r\n][ \t\r\n]*/g
  if (!structured) return text.replace(lineBreaks, ' ')

  let quote: '"' | "'" | null = null
  let escaped = false
  function advance(char: string) {
    if (quote) {
      if (escaped) escaped = false
      else if (char === '\\') escaped = true
      else if (char === quote) quote = null
    } else if (char === '"' || char === "'") quote = char
  }
  for (const char of before) advance(char)

  let result = ''
  for (let i = 0; i < text.length; i++) {
    const char = text[i]
    if (quote && (char === '\r' || char === '\n')) {
      // An existing backslash already escapes this received newline.
      result += `${escaped ? '' : '\\'}${char === '\r' ? 'r' : 'n'}`
      escaped = false
    } else if (!quote && /[ \t\r\n]/.test(char)) {
      const start = i
      while (i + 1 < text.length && /[ \t\r\n]/.test(text[i + 1])) i++
      result += text.slice(start, i + 1).replace(lineBreaks, ' ')
    } else {
      result += char
      advance(char)
    }
  }
  return result
}

export const multilineQueryPaste = EditorView.domEventHandlers({
  paste(event, view) {
    const text = event.clipboardData?.getData('text/plain')
    if (!text || !/[\r\n]/.test(text)) return false
    const { state } = view
    const changes = state.changeByRange(range => {
      const insert = normalizeQueryPaste(
        text,
        state.sliceDoc(0, range.from),
        state.sliceDoc(range.to)
      )
      return {
        changes: { from: range.from, to: range.to, insert },
        range: EditorSelection.cursor(range.from + insert.length),
      }
    })
    view.dispatch(changes, {
      userEvent: 'input.paste',
      annotations: isolateHistory.of('full'),
      scrollIntoView: true,
    })
    event.preventDefault()
    return true
  },
})

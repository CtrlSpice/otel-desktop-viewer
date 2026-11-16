import { StreamLanguage } from '@codemirror/language'
import { shell } from '@codemirror/legacy-modes/mode/shell'
import { highlightTree } from '@lezer/highlight'
import { editorHighlightStyle } from './editor-highlighting'

export const shellLanguage = StreamLanguage.define(shell)

/** Token spans retain the original text, including whitespace and incomplete input. */
export function shellTokens(text: string) {
  const tokens: { text: string; classes: string }[] = []
  let position = 0
  highlightTree(
    shellLanguage.parser.parse(text),
    editorHighlightStyle,
    (from, to, classes) => {
      if (from > position)
        tokens.push({ text: text.slice(position, from), classes: '' })
      tokens.push({ text: text.slice(from, to), classes })
      position = to
    }
  )
  if (position < text.length)
    tokens.push({ text: text.slice(position), classes: '' })
  return tokens
}

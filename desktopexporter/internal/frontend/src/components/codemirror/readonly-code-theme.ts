import { EditorView } from '@codemirror/view'
import { HighlightStyle, syntaxHighlighting } from '@codemirror/language'
import { tags as t } from '@lezer/highlight'
import { editorColors as c } from './editor-colors'

const shellHighlightStyle = HighlightStyle.define([
  { tag: t.comment, color: c.subtle, fontStyle: 'italic' },
  { tag: t.string, color: c.gold },
  { tag: t.number, color: c.rose },
  { tag: t.variableName, color: c.foam },
  { tag: t.keyword, color: c.iris },
  { tag: t.operator, color: c.subtle },
  { tag: t.meta, color: c.subtle },
  { tag: t.name, color: c.text },
])

export const readonlyCodeEditorTheme = EditorView.theme({
  '&': {
    fontSize: '13px',
    fontFamily: '"Atkinson Hyperlegible Mono", ui-monospace, monospace',
    backgroundColor: c.base,
    color: c.text,
  },
  '&.cm-focused': {
    outline: 'none',
  },
  '.cm-scroller': {
    overflow: 'auto',
    fontFamily: 'inherit',
  },
  '.cm-content': {
    padding: '0.75rem 1rem 1rem',
    caretColor: 'transparent',
  },
  '.cm-cursor, .cm-dropCursor': {
    display: 'none',
  },
  '.cm-selectionBackground, &.cm-focused .cm-selectionBackground': {
    backgroundColor: `${c.overlay} !important`,
  },
  '.cm-activeLine': {
    backgroundColor: 'color-mix(in oklab, var(--color-base-300) 35%, transparent)',
  },
  '.cm-gutters': {
    display: 'none',
  },
})

export const readonlyCodeTheme = [
  readonlyCodeEditorTheme,
  syntaxHighlighting(shellHighlightStyle),
]

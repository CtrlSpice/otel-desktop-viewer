import { EditorView } from '@codemirror/view'
import { syntaxHighlighting } from '@codemirror/language'
import { editorHighlightStyle } from '@/components/shared/utils/editor-highlighting'
import { stringSyntaxHighlighting } from '@/components/shared/utils/string-syntax-highlighting'

import { editorColors as qc } from '@/components/shared/utils/editor-colors'

export const queryEditorTheme = EditorView.theme({
  '&': {
    fontSize: 'var(--text-base)',
    fontFamily: 'var(--font-sans)',
    backgroundColor: 'transparent',
    color: qc.text,
  },
  '.cm-scroller': {
    fontFamily: 'inherit',
  },
  '.cm-content': {
    caretColor: qc.cursor,
    padding: '0',
  },
  '.cm-cursor, .cm-dropCursor': {
    borderLeftColor: qc.cursor,
  },
  '&.cm-focused': {
    outline: 'none',
  },
  '.cm-selectionBackground': {
    backgroundColor: `${qc.highlightMed} !important`,
  },
  '.cm-activeLine': {
    backgroundColor: 'transparent',
  },
  '.cm-placeholder': {
    color: qc.subtle,
    fontSize: 'var(--text-base)',
    fontFamily: 'var(--font-sans)',
    fontStyle: 'normal',
    lineHeight: 'var(--text-base--line-height)',
  },
  // Lint diagnostics (render inline in .cm-editor, so EditorView.theme works)
  '.cm-diagnostic-error': {
    borderLeft: `3px solid ${qc.error}`,
    backgroundColor: 'color-mix(in oklab, var(--color-error) 12%, transparent)',
    padding: '4px 8px',
    marginLeft: '4px',
  },
  '.cm-lintRange-error': {
    backgroundImage: 'none',
    textDecoration: `underline wavy ${qc.error}`,
    textUnderlineOffset: '3px',
  },
})

export const queryTheme = [
  queryEditorTheme,
  syntaxHighlighting(editorHighlightStyle),
  stringSyntaxHighlighting,
]

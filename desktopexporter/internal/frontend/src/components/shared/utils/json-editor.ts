import { EditorState } from '@codemirror/state'
import { EditorView, keymap } from '@codemirror/view'
import { json } from '@codemirror/lang-json'
import {
  codeFolding,
  foldKeymap,
  syntaxHighlighting,
} from '@codemirror/language'
import { search, searchKeymap } from '@codemirror/search'
import { editorHighlightStyle } from './editor-highlighting'
import { jsonWrapping } from './json-wrapping'
import { jsonGuides } from './json-guides'

/** One read-only configuration shared by all JSON detail editors. */
export const jsonEditorExtensions = [
  EditorState.readOnly.of(true),
  EditorView.editable.of(false),
  json(),
  jsonWrapping,
  jsonGuides,
  syntaxHighlighting(editorHighlightStyle),
  codeFolding(),
  search({ top: true }),
  keymap.of([...searchKeymap, ...foldKeymap]),
  EditorView.theme({
    '&': {
      height: 'auto',
      color: 'var(--color-base-content)',
      backgroundColor: 'var(--color-base-100)',
      fontSize: 'inherit',
    },
    '.cm-scroller': {
      overflow: 'auto',
      fontFamily: 'inherit',
      lineHeight: 'inherit',
    },
    '.cm-content': { padding: '0' },
    '.cm-line': { paddingLeft: '0' },
    '.cm-foldPlaceholder': {
      backgroundColor: 'transparent',
      border: 'none',
      color: 'var(--color-subtle)',
    },
    '.cm-panels': {
      backgroundColor: 'var(--color-base-100)',
      color: 'var(--color-base-content)',
    },
    '.cm-textfield': {
      backgroundColor: 'var(--color-base-100)',
      color: 'var(--color-base-content)',
    },
    '.cm-searchMatch': { backgroundColor: 'var(--color-highlight-low)' },
    '.cm-searchMatch-selected': { outline: '1px solid var(--color-primary)' },
    '.cm-selectionBackground': {
      backgroundColor: 'var(--color-highlight-med) !important',
    },
    '&.cm-focused': { outline: 'none' },
  }),
]

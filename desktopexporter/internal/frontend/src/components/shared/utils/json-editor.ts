import { EditorState } from '@codemirror/state'
import { EditorView, keymap } from '@codemirror/view'
import { json } from '@codemirror/lang-json'
import {
  codeFolding,
  foldKeymap,
  bracketMatching,
  syntaxHighlighting,
} from '@codemirror/language'
import { search, searchKeymap } from '@codemirror/search'
import { editorHighlightStyle } from './editor-highlighting'
import { jsonWrapping } from './json-wrapping'
import { jsonFoldGroups } from './json-fold-groups'
import { editorColors as c } from './editor-colors'
import { stringSyntaxHighlighting } from './string-syntax-highlighting'

/** One read-only configuration shared by all JSON detail editors. */
export const jsonEditorExtensions = [
  EditorState.readOnly.of(true),
  EditorView.editable.of(false),
  json(),
  jsonWrapping,
  syntaxHighlighting(editorHighlightStyle),
  stringSyntaxHighlighting,
  codeFolding(),
  bracketMatching(),
  jsonFoldGroups,
  search({ top: true }),
  keymap.of([...searchKeymap, ...foldKeymap]),
  EditorView.theme({
    '&': {
      height: 'auto',
      color: 'var(--color-base-content)',
      backgroundColor: 'var(--json-editor-background, var(--color-base-100))',
      fontSize: 'inherit',
    },
    '.cm-scroller': {
      overflow: 'auto',
      fontFamily: 'inherit',
      lineHeight: 'inherit',
    },
    '.cm-content': { padding: '0' },
    '.cm-line': { paddingLeft: '0' },
    '.cm-gutters': {
      backgroundColor: 'transparent',
      border: 'none',
      color: 'var(--color-subtle)',
    },
    '.cm-foldGutter .cm-gutterElement': { padding: '0' },
    '.json-fold-marker': {
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      width: 'var(--waterfall-gutter-col)',
      height: '1lh',
      cursor: 'pointer',
    },
    '.json-fold-marker:hover': {
      backgroundColor: 'var(--color-highlight-low)',
    },
    '.json-fold-icon': { width: '1em', height: '1em' },
    '.json-fold-marker[data-open="true"] .json-fold-icon': {
      transform: 'rotate(90deg)',
    },
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
      fontFamily: 'var(--font-sans)',
      backgroundColor: 'var(--color-input)',
      border: 'none',
      color: 'var(--color-base-content)',
      caretColor: c.cursor,
    },
    '.cm-searchMatch': { backgroundColor: c.searchMatch },
    '&.cm-focused .cm-matchingBracket': {
      backgroundColor: c.bracketFill,
      color: 'inherit',
      outline: `1px solid ${c.bracketBorder}`,
    },
    '.cm-searchMatch-selected': { backgroundColor: c.searchMatchSelected },
    '.cm-selectionBackground': {
      backgroundColor: 'var(--color-highlight-med) !important',
    },
    '&.cm-focused': { outline: 'none' },
  }),
]

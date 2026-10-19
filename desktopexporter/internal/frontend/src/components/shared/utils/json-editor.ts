import { EditorState } from '@codemirror/state'
import { EditorView, keymap } from '@codemirror/view'
import { json } from '@codemirror/lang-json'
import {
  foldGutter,
  foldKeymap,
  HighlightStyle,
  syntaxHighlighting,
} from '@codemirror/language'
import { tags } from '@lezer/highlight'
import { search, searchKeymap } from '@codemirror/search'
import { createJsonFoldCaret } from './json-fold-caret'

/** One read-only configuration shared by all JSON detail editors. */
export const jsonEditorExtensions = [
  EditorState.readOnly.of(true),
  EditorView.editable.of(false),
  json(),
  syntaxHighlighting(
    HighlightStyle.define([
      { tag: tags.propertyName, color: 'var(--detail-code-key)' },
      { tag: tags.string, color: 'var(--detail-code-string)' },
      { tag: tags.number, color: 'var(--detail-code-number)' },
      { tag: [tags.bool, tags.null], color: 'var(--detail-code-literal)' },
      { tag: tags.punctuation, color: 'var(--detail-code-key)' },
    ])
  ),
  foldGutter({
    markerDOM: open => {
      const target = document.createElement('span')
      target.className = 'json-fold-target json-fold-control'
      target.title = open ? 'Fold line' : 'Unfold line'
      target.dataset.expanded = String(open)
      target.append(createJsonFoldCaret())
      return target
    },
  }),
  search({ top: true }),
  keymap.of([...searchKeymap, ...foldKeymap]),
  EditorView.theme({
    '&': {
      height: 'auto',
      color: 'var(--color-base-content)',
      backgroundColor: 'transparent',
      fontSize: 'inherit',
    },
    '.cm-scroller': {
      overflow: 'auto',
      fontFamily: 'inherit',
      lineHeight: 'inherit',
    },
    '.cm-content': { padding: '8px 0' },
    '.cm-line': { paddingLeft: '0' },
    '.cm-gutters': {
      backgroundColor: 'transparent',
      color: 'var(--color-subtle)',
      border: 'none',
    },
    '.cm-foldGutter .cm-gutterElement': {
      minWidth: '1.25rem',
      textAlign: 'center',
    },
    '.json-fold-target': { display: 'block', width: '100%', height: '1lh' },
    '.cm-foldPlaceholder': {
      backgroundColor: 'transparent',
      border: 'none',
      color: 'var(--color-subtle)',
    },
    '.cm-panels': {
      backgroundColor: 'var(--color-base-200)',
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

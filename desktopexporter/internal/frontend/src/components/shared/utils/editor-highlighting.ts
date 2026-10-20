import { HighlightStyle } from '@codemirror/language'
import { tags } from '@lezer/highlight'
import { editorColors as c } from './editor-colors'

// Rosé Pine's official VS Code syntax roles, adapted to Lezer tags.
// Source and scope mapping: src/themes/README.md.
export const editorHighlightStyle = HighlightStyle.define([
  { tag: tags.name, color: c.text },
  { tag: tags.propertyName, color: c.pine },
  { tag: tags.string, color: c.text },
  { tag: tags.literal, color: c.literal },
  { tag: [tags.keyword, tags.operator], color: c.pine },
  { tag: tags.punctuation, color: c.subtle },
  { tag: tags.escape, color: c.pine },
  { tag: tags.comment, color: c.subtle, fontStyle: 'italic' },
  { tag: tags.invalid, color: c.love },
])

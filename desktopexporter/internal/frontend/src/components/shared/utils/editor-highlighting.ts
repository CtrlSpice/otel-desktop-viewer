import { HighlightStyle } from '@codemirror/language'
import { tags } from '@lezer/highlight'
import { editorColors as c } from './editor-colors'

// Madoromi's code guide, with neutral string bodies and literal values.
// Shared readable shades come from the theme generator. See src/themes/README.md.
export const editorHighlightStyle = HighlightStyle.define([
  { tag: tags.name, color: c.text },
  { tag: tags.propertyName, color: c.property },
  { tag: tags.string, color: c.string },
  // Lezer classifies null and atoms as keywords rather than literals.
  {
    tag: [tags.literal, tags.null, tags.atom, tags.constant(tags.variableName)],
    color: c.literal,
  },
  { tag: tags.keyword, color: c.keyword },
  { tag: [tags.operator, tags.operatorKeyword], color: c.operator },
  { tag: tags.punctuation, color: c.punctuation },
  { tag: [tags.escape, tags.regexp, tags.tagName], color: c.escape },
  {
    tag: [
      tags.character,
      tags.constant(tags.propertyName),
      tags.macroName,
      tags.processingInstruction,
    ],
    color: c.character,
  },
  {
    tag: [tags.function(tags.variableName), tags.function(tags.propertyName)],
    color: c.function,
  },
  { tag: tags.standard(tags.name), color: c.builtin },
  { tag: tags.self, color: c.builtin, fontStyle: 'italic' },
  { tag: [tags.typeName, tags.className, tags.attributeName], color: c.type },
  { tag: tags.namespace, color: c.namespace },
  { tag: tags.annotation, color: c.annotation },
  { tag: tags.labelName, color: c.label },
  { tag: tags.comment, color: c.comment, fontStyle: 'italic' },
  { tag: tags.invalid, color: c.error },
])

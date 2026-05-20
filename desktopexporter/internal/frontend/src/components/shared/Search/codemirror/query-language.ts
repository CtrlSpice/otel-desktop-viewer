import { LRLanguage, LanguageSupport } from '@codemirror/language'
import { styleTags, tags as t } from '@lezer/highlight'
import { parser } from './query.parser'

const queryHighlighting = styleTags({
  Field: t.propertyName,
  Operator: t.compareOperator,
  KeywordOperator: t.operatorKeyword,
  LogicalOp: t.logicOperator,
  QuotedString: t.string,
  Value: t.literal,
  Null: t.null,
  'Array/[ Array/]': t.squareBracket,
  'Group/( Group/)': t.paren,
})

export const queryLanguage = LRLanguage.define({
  name: 'otel-query',
  parser: parser.configure({
    props: [queryHighlighting],
  }),
  languageData: {
    closeBrackets: { brackets: ['(', '[', '"', "'"] },
  },
})

export function queryLanguageSupport() {
  return new LanguageSupport(queryLanguage)
}

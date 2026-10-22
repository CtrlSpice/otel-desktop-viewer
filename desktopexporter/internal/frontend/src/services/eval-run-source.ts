import { jsonLanguage } from '@codemirror/lang-json'
import type { SyntaxNode } from '@lezer/common'
import {
  evalProjectionLimits,
  type EvalSourceDataset,
} from '@/components/evals/promptfoo-adapter'
import type { EvalRunSourcePage } from './eval-run-types'

/** Assemble a complete selected run for the bounded existing adapter, preserving
 * every received numeric lexeme. Fetch only this run; never JSON.stringify its
 * parsed context or source rows. Large-run table rendering can use pages directly. */
export function assembleEvalRunSource(
  pages: readonly EvalRunSourcePage[]
): EvalSourceDataset {
  const first = pages[0]
  if (!first) throw new Error('No selected-run source pages were received.')
  const summary = first.summary
  if (summary.repCount > evalProjectionLimits.rows)
    throw new Error(
      'Selected run exceeds the current 5,000-row display adapter limit.'
    )
  if (pages.at(-1)?.hasMore)
    throw new Error('Selected-run source pages are incomplete.')
  const rowOrdinals: number[] = []
  const rows: string[] = []
  let characters = first.contextJSON.length
  for (const page of pages) {
    if (
      page.summary.id !== summary.id ||
      page.contextJSON !== first.contextJSON
    )
      throw new Error('Source pages belong to different runs or contexts.')
    for (const row of page.rows) {
      if (
        !Number.isSafeInteger(row.ordinal) ||
        row.ordinal < 0 ||
        row.ordinal <= (rowOrdinals.at(-1) ?? -1)
      )
        throw new Error('Source row ordinals must be strictly increasing.')
      characters += row.json.length + 1
      if (characters > evalProjectionLimits.payloadCharacters)
        throw new Error(
          'Selected run exceeds the current display adapter text limit.'
        )
      rowOrdinals.push(row.ordinal)
      rows.push(row.json)
    }
  }
  if (rows.length !== summary.repCount)
    throw new Error(
      'Selected-run source pages do not contain the complete recorded run.'
    )
  let node: SyntaxNode | null = jsonLanguage.parser.parse(first.contextJSON)
    .topNode.firstChild
  const path = ['results', 'results']
  for (const key of path) {
    const property: SyntaxNode | undefined = node
      ?.getChildren('Property')
      .find(property => {
        const name = property.getChild('PropertyName')
        return (
          name &&
          JSON.parse(first.contextJSON.slice(name.from, name.to)) === key
        )
      })
    node = property?.lastChild ?? null
  }
  if (
    node?.name !== 'Array' ||
    first.contextJSON.slice(node.from, node.to) !== '[]'
  )
    throw new Error(
      'Selected-run source context has no empty producer result array.'
    )
  return {
    id: summary.datasetID,
    name: summary.evaluation,
    source: summary.source,
    format: summary.format,
    payload:
      first.contextJSON.slice(0, node.from) +
      '[' +
      rows.join(',') +
      ']' +
      first.contextJSON.slice(node.to),
    rowOrdinals,
  }
}

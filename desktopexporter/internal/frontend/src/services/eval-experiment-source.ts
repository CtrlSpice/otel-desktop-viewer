import { jsonLanguage } from '@codemirror/lang-json'
import type { SyntaxNode } from '@lezer/common'
import {
  evalProjectionLimits,
  type EvalSourceDataset,
} from '@/components/evals/promptfoo-adapter'
import type { EvalExperimentSourcePage } from './eval-experiment-types'
import { evalAPI } from './eval-service'

/** Stop pagination before retaining more than the display adapter can consume. */
export async function loadEvalExperimentSource(
  id: string,
  signal: AbortSignal
) {
  let selected: EvalExperimentSourcePage | undefined
  let after = -1
  let characters = 0
  let rows = 0
  while (true) {
    const page = await evalAPI.getEvalExperimentPage(id, after, 100, signal)
    signal.throwIfAborted()
    if (page.summary.resultCount > evalProjectionLimits.rows)
      throw new Error(
        `This experiment has ${page.summary.resultCount.toLocaleString()} results and exceeds the current 5,000-row display limit.`
      )
    const first = selected
    if (
      first &&
      (page.summary.id !== first.summary.id ||
        page.contextJSON !== first.contextJSON)
    )
      throw new Error(
        'Source pages belong to different experiments or contexts.'
      )
    const contextJSON = first?.contextJSON ?? page.contextJSON
    if (!first) characters = contextJSON.length
    for (const row of page.rows) characters += row.json.length + 1
    if (characters > evalProjectionLimits.payloadCharacters)
      throw new Error(
        'Selected experiment exceeds the current display adapter text limit.'
      )
    rows += page.rows.length
    if (rows > evalProjectionLimits.rows)
      throw new Error(
        'Selected experiment exceeds the current 5,000-row display adapter limit.'
      )
    // Keep one summary/context; later responses contribute only rows and cursor state.
    if (selected) {
      selected.rows.push(...page.rows)
      selected.nextAfter = page.nextAfter
      selected.hasMore = page.hasMore
    } else selected = { ...page, rows: [...page.rows] }
    if (!page.hasMore) break
    if (page.nextAfter <= after)
      throw new Error('Selected-experiment source pagination did not advance.')
    after = page.nextAfter
  }
  return {
    summary: selected.summary,
    source: assembleEvalExperimentSource([selected]),
  }
}

/** Assemble a complete selected experiment for the bounded existing adapter, preserving
 * every received numeric lexeme. Fetch only this experiment; never JSON.stringify its
 * parsed context or source rows. Large-experiment table rendering can use pages directly. */
export function assembleEvalExperimentSource(
  pages: readonly EvalExperimentSourcePage[]
): EvalSourceDataset {
  const first = pages[0]
  if (!first)
    throw new Error('No selected-experiment source pages were received.')
  const summary = first.summary
  if (summary.resultCount > evalProjectionLimits.rows)
    throw new Error(
      'Selected experiment exceeds the current 5,000-row display adapter limit.'
    )
  if (pages.at(-1)?.hasMore)
    throw new Error('Selected-experiment source pages are incomplete.')
  const rowOrdinals: number[] = []
  const rows: string[] = []
  let characters = first.contextJSON.length
  for (const page of pages) {
    if (
      page.summary.id !== summary.id ||
      page.contextJSON !== first.contextJSON
    )
      throw new Error(
        'Source pages belong to different experiments or contexts.'
      )
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
          'Selected experiment exceeds the current display adapter text limit.'
        )
      rowOrdinals.push(row.ordinal)
      rows.push(row.json)
    }
  }
  if (rows.length !== summary.resultCount)
    throw new Error(
      'Selected-experiment source pages do not contain the complete recorded experiment.'
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
      'Selected-experiment source context has no empty producer result array.'
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

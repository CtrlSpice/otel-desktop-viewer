import { jsonLanguage } from '@codemirror/lang-json'
import type { SyntaxNode } from '@lezer/common'
import type {
  EvalSourceDataset,
  EvalSourceRowReference,
  EvalRunTextSource,
  EvalSourceTextField,
} from './promptfoo-adapter'

export type EvalSourceTextLoader = (signal: AbortSignal) => Promise<string>
export type EvalSourceFieldCopy = (
  repID: string,
  field: EvalSourceTextField
) => EvalSourceTextLoader | undefined
export type EvalSourceRunCopy = (
  runID: string,
  field: 'name' | 'evaluation' | 'source' | 'prompt'
) => EvalSourceTextLoader | undefined

/** Read a validated retained dataset, never its display projection. Strings are
 * decoded prose; all other values retain their original JSON subtree/lexemes.
 * Duplicate object keys follow JSON.parse's last-key selection in the adapter. */
export function evalSourceFieldText(payload: string, pointer: string): string {
  let node: SyntaxNode | null =
    jsonLanguage.parser.parse(payload).topNode.firstChild
  for (const segment of pointer.split('/').slice(1)) {
    const key = segment.replace(/~1/g, '/').replace(/~0/g, '~')
    let next: SyntaxNode | null = null
    if (node?.name === 'Object') {
      for (const property of node.getChildren('Property')) {
        const name = property.getChild('PropertyName')!
        if (JSON.parse(payload.slice(name.from, name.to)) === key)
          next = property.lastChild
      }
    } else if (node?.name === 'Array') {
      let index = 0
      for (let child = node.firstChild; child; child = child.nextSibling) {
        if (['[', ']', ','].includes(child.name)) continue
        if (String(index++) === key) {
          next = child
          break
        }
      }
    }
    node = next
    if (!node) throw new Error('Source field is missing.')
  }
  if (!node) throw new Error('Source field is missing.')
  const raw = payload.slice(node.from, node.to)
  return node.name === 'String' ? (JSON.parse(raw) as string) : raw
}

/** Capture the immutable payload and row mapping. Parse on demand for Copy or
 * full assessment details, without replacing the retained payload. */
export function evalSourceFieldCopy(
  payload: string,
  rows: EvalSourceRowReference[]
): EvalSourceFieldCopy {
  const sources = new Map(rows.map(row => [row.repID, row.fields]))
  return (repID, field) => {
    const pointer = sources.get(repID)?.[field]
    if (pointer === undefined) return undefined
    return async signal => {
      signal.throwIfAborted()
      return evalSourceFieldText(payload, pointer)
    }
  }
}

/** Evaluation/source are native text. The run name is a derived joined label:
 * reconstruct its established formatting from full operands, not a fictional
 * source field. Scores, durations and costs keep their formatted copy semantics. */
export function evalSourceRunCopy(
  dataset: EvalSourceDataset,
  sources: Record<string, EvalRunTextSource>
): EvalSourceRunCopy {
  return (runID, field) => {
    const source = sources[runID]
    if (!source) return undefined
    if (field === 'prompt' && source.promptLabel === undefined) return undefined
    return async signal => {
      signal.throwIfAborted()
      if (field === 'source') return dataset.source
      if (field === 'prompt')
        return evalSourceFieldText(dataset.payload, source.promptLabel!)
      const evaluation = source.evaluation
        ? evalSourceFieldText(dataset.payload, source.evaluation)
        : dataset.name
      if (field === 'evaluation') return evaluation
      return source.providerName
        ? evalSourceFieldText(dataset.payload, source.providerName)
        : evaluation
    }
  }
}

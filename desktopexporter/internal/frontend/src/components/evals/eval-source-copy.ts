import { jsonLanguage } from '@codemirror/lang-json'
import { evalSourceNode } from './eval-source-node'
import type {
  EvalSourceDataset,
  EvalSourceRowReference,
  EvalExperimentTextSource,
  EvalSourceTextField,
} from './promptfoo-adapter'

export type EvalSourceTextLoader = (signal: AbortSignal) => Promise<string>
export type EvalSourceFieldCopy = (
  resultID: string,
  field: EvalSourceTextField
) => EvalSourceTextLoader | undefined
export type EvalSourceExperimentCopy = (
  experimentID: string,
  field: 'name' | 'evaluation' | 'source' | 'prompt'
) => EvalSourceTextLoader | undefined

/** Read a validated retained dataset, never its display projection. Strings are
 * decoded prose; all other values retain their original JSON subtree/lexemes.
 * Duplicate object keys follow JSON.parse's last-key selection in the adapter. */
export function evalSourceFieldText(payload: string, pointer: string): string {
  const node = evalSourceNode(
    payload,
    pointer,
    jsonLanguage.parser.parse(payload).topNode.firstChild
  )
  const raw = payload.slice(node.from, node.to)
  if (node.name !== 'String') return raw
  const decoded: unknown = JSON.parse(raw)
  // oxlint-disable-next-line anti-slop/no-runtime-typeof -- Validate the decoded String subtree; non-string subtrees return their original source slice above.
  if (typeof decoded !== 'string') throw new Error('Invalid source string.')
  return decoded
}

/** Capture the immutable payload and row mapping. Parse on demand for Copy or
 * full assessment details, without replacing the retained payload. */
export function evalSourceFieldCopy(
  payload: string,
  rows: EvalSourceRowReference[]
): EvalSourceFieldCopy {
  const sources = new Map(rows.map(row => [row.resultID, row.fields]))
  return (resultID, field) => {
    const pointer = sources.get(resultID)?.[field]
    if (pointer === undefined) return undefined
    return async signal => {
      signal.throwIfAborted()
      return evalSourceFieldText(payload, pointer)
    }
  }
}

/** Evaluation/source are native text. The experiment name is a derived joined label:
 * reconstruct its established formatting from full operands, not a fictional
 * source field. Scores, durations and costs keep their formatted copy semantics. */
export function evalSourceExperimentCopy(
  dataset: EvalSourceDataset,
  sources: Record<string, EvalExperimentTextSource>
): EvalSourceExperimentCopy {
  return (experimentID, field) => {
    const source = sources[experimentID]
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

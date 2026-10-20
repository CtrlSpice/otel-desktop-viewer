import { jsonLanguage } from '@codemirror/lang-json'
import type { SyntaxNode } from '@lezer/common'
import type {
  EvalSourceSnapshot,
  SnapshotRowReference,
  SnapshotRunTextSource,
  SnapshotTextField,
} from './snapshot-adapters'

export type SnapshotTextLoader = (signal: AbortSignal) => Promise<string>
export type SnapshotFieldCopy = (
  attemptID: string,
  field: SnapshotTextField
) => SnapshotTextLoader | undefined
export type SnapshotRunCopy = (
  runID: string,
  field: 'name' | 'evaluation' | 'source'
) => SnapshotTextLoader | undefined

/** Read a validated retained snapshot, never its display projection. Strings are
 * decoded prose; all other values retain their original JSON subtree/lexemes.
 * Duplicate object keys follow JSON.parse's last-key selection in the adapter. */
export function snapshotFieldText(payload: string, pointer: string): string {
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

/** Capture the immutable payload and row mapping. Parse only on explicit Copy. */
export function snapshotFieldCopy(
  payload: string,
  rows: SnapshotRowReference[]
): SnapshotFieldCopy {
  const sources = new Map(rows.map(row => [row.attemptID, row.fields]))
  return (attemptID, field) => {
    const pointer = sources.get(attemptID)?.[field]
    if (pointer === undefined) return undefined
    return async signal => {
      signal.throwIfAborted()
      return snapshotFieldText(payload, pointer)
    }
  }
}

/** Evaluation/source are native text. The run name is a derived joined label:
 * reconstruct its established formatting from full operands, not a fictional
 * source field. Scores, durations and costs keep their formatted copy semantics. */
export function snapshotRunCopy(
  snapshot: EvalSourceSnapshot,
  sources: Record<string, SnapshotRunTextSource>
): SnapshotRunCopy {
  return (runID, field) => {
    const source = sources[runID]
    if (!source) return undefined
    return async signal => {
      signal.throwIfAborted()
      if (field === 'source') return snapshot.source
      const evaluation = source.evaluation
        ? snapshotFieldText(snapshot.payload, source.evaluation)
        : snapshot.name
      if (field === 'evaluation') return evaluation
      return [
        evaluation,
        source.providerName
          ? snapshotFieldText(snapshot.payload, source.providerName)
          : undefined,
        source.promptLabel
          ? snapshotFieldText(snapshot.payload, source.promptLabel)
          : undefined,
      ]
        .filter(Boolean)
        .join(' · ')
    }
  }
}

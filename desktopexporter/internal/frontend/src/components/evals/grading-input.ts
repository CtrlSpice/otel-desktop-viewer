import { evalSourceFieldText } from './eval-source-copy'

export type GradingCriterion = {
  id?: string
  text: string
  scoring: Record<string, string>
}
export type GradingInput = {
  instructions: string
  question: string
  criteria: GradingCriterion[]
  reference?: string
  viewerSkill?: string
  evidence?: string
  artifact?: string
}

function record(value: unknown): value is object {
  return value !== null && typeof value === 'object' && !Array.isArray(value)
}

type GradingPacket = {
  question: string
  criteria: { text: string; scoring: Record<string, string>; id?: unknown }[]
  reference?: unknown
  viewerSkill?: unknown
  evidence?: unknown
  artifact?: unknown
}

function isGradingText(value: unknown): value is string {
  return typeof value === 'string'
}

function isGradingPacket(value: unknown): value is GradingPacket {
  if (
    !record(value) ||
    !('question' in value) ||
    typeof value.question !== 'string' ||
    !('criteria' in value) ||
    !Array.isArray(value.criteria) ||
    !value.criteria.length
  )
    return false
  for (const criterion of value.criteria) {
    if (
      !record(criterion) ||
      !('text' in criterion) ||
      typeof criterion.text !== 'string' ||
      !('scoring' in criterion) ||
      !record(criterion.scoring)
    )
      return false
    for (const description of Object.values(criterion.scoring))
      if (typeof description !== 'string') return false
  }
  return true
}

/** Recognize the explicit rubric/evidence packet, not arbitrary prose or tool output.
 * The original grader input remains authoritative and copyable. */
export function parseGradingInput(raw: string): GradingInput | undefined {
  const start = raw.trimStart().startsWith('{')
    ? raw.indexOf('{')
    : raw.lastIndexOf('\n{') + 1
  if (start < 0 || raw[start] !== '{') return undefined
  const json = raw.slice(start)
  try {
    const packet: unknown = JSON.parse(json)
    if (!isGradingPacket(packet)) return undefined
    const criteria: GradingCriterion[] = []
    for (const criterion of packet.criteria) {
      const scoring: Record<string, string> = {}
      for (const [label, description] of Object.entries(criterion.scoring)) {
        Object.defineProperty(scoring, label, {
          value: description,
          enumerable: true,
        })
      }
      criteria.push({
        id: isGradingText(criterion.id) ? criterion.id : undefined,
        text: criterion.text,
        scoring,
      })
    }
    return {
      instructions: raw.slice(0, start).trim(),
      question: packet.question,
      criteria,
      reference: isGradingText(packet.reference) ? packet.reference : undefined,
      viewerSkill: isGradingText(packet.viewerSkill)
        ? packet.viewerSkill
        : undefined,
      evidence:
        packet.evidence === undefined
          ? undefined
          : evalSourceFieldText(json, '/evidence'),
      artifact:
        packet.artifact === undefined
          ? undefined
          : evalSourceFieldText(json, '/artifact'),
    }
  } catch {
    return undefined
  }
}

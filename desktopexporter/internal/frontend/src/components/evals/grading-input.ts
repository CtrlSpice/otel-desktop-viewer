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

function record(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === 'object' && !Array.isArray(value)
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
    if (
      !record(packet) ||
      typeof packet.question !== 'string' ||
      !Array.isArray(packet.criteria) ||
      !packet.criteria.length
    )
      return undefined
    const criteria: GradingCriterion[] = []
    for (const criterion of packet.criteria) {
      if (
        !record(criterion) ||
        typeof criterion.text !== 'string' ||
        !record(criterion.scoring)
      )
        return undefined
      const scoring: Record<string, string> = {}
      for (const [label, description] of Object.entries(criterion.scoring)) {
        if (typeof description !== 'string') return undefined
        Object.defineProperty(scoring, label, {
          value: description,
          enumerable: true,
        })
      }
      criteria.push({
        id: typeof criterion.id === 'string' ? criterion.id : undefined,
        text: criterion.text,
        scoring,
      })
    }
    return {
      instructions: raw.slice(0, start).trim(),
      question: packet.question,
      criteria,
      reference:
        typeof packet.reference === 'string' ? packet.reference : undefined,
      viewerSkill:
        typeof packet.viewerSkill === 'string' ? packet.viewerSkill : undefined,
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

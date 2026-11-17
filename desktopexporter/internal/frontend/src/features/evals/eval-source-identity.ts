import { jsonLanguage } from '@codemirror/lang-json'
import type { SyntaxNode } from '@lezer/common'
import { evalSourceNode } from './eval-source-node'

/** Canonical decimal coefficient/exponent, without converting through Number.
 * Equivalent spellings group together; signed zero remains distinct. */
function numberIdentity(raw: string): string {
  const match = /^(-?)(\d+)(?:\.(\d+))?(?:[eE]([+-]?\d+))?$/.exec(raw)
  if (!match) throw new Error('Invalid source number.')
  const [, sign, integer, fraction = '', exponent = '0'] = match
  const digits = (integer + fraction).replace(/^0+/, '')
  if (!digits) return sign + '0'
  const coefficient = digits.replace(/0+$/, '')
  const power =
    BigInt(exponent) -
    BigInt(fraction.length) +
    BigInt(digits.length - coefficient.length)
  return sign + coefficient + 'e' + power
}

function arrayValues(node: SyntaxNode): SyntaxNode[] {
  const values: SyntaxNode[] = []
  for (let child = node.firstChild; child; child = child.nextSibling)
    if (!['[', ']', ','].includes(child.name)) values.push(child)
  return values
}

function canonical(payload: string, node: SyntaxNode): string {
  switch (node.name) {
    case 'Object': {
      const members = new Map<string, SyntaxNode>()
      for (const property of node.getChildren('Property')) {
        const key = property.getChild('PropertyName')!
        // Canonical JSON string keys equate escaped spellings and retain last-key selection.
        members.set(
          JSON.stringify(JSON.parse(payload.slice(key.from, key.to))),
          property.lastChild!
        )
      }
      return (
        '{' +
        [...members]
          .sort(([a], [b]) => (a < b ? -1 : a > b ? 1 : 0))
          .map(([key, value]) => key + ':' + canonical(payload, value))
          .join(',') +
        '}'
      )
    }
    case 'Array':
      return (
        '[' +
        arrayValues(node)
          .map(value => canonical(payload, value))
          .join(',') +
        ']'
      )
    case 'String':
      return JSON.stringify(JSON.parse(payload.slice(node.from, node.to)))
    case 'Number':
      return numberIdentity(payload.slice(node.from, node.to))
    case 'True':
    case 'False':
    case 'Null':
      return payload.slice(node.from, node.to)
    default:
      throw new Error('Invalid source value for test-case identity.')
  }
}

/** Build once for an already validated/bounded export. Row indexing avoids a
 * repeated scan of the complete result array for each test definition. */
export function evalSourceIdentities(payload: string) {
  const root = jsonLanguage.parser.parse(payload).topNode.firstChild
  const rows = arrayValues(evalSourceNode(payload, '/results/results', root))
  return (row: number, pointer: string): string =>
    canonical(payload, evalSourceNode(payload, pointer, rows[row] ?? null))
}

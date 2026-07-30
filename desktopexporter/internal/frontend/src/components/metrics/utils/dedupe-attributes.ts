import type { Attribute, Attributes } from '@/types/api-types'

/** One entry per key; later duplicates win but first-seen order is kept. */
export function dedupeAttributes(attrs: readonly Attribute[]): Attributes {
  const byKey = new Map<string, Attribute>()
  const order: string[] = []
  for (const a of attrs) {
    if (!byKey.has(a.key)) order.push(a.key)
    byKey.set(a.key, a)
  }
  return order.map(k => byKey.get(k)!)
}

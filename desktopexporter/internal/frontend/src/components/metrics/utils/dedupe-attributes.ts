import type { Attribute, Attributes } from '@/types/api-types'

/** Attribute entries are lossless: duplicate keys can carry distinct typed values. */
export function dedupeAttributes(attrs: readonly Attribute[]): Attributes {
  return [...attrs]
}

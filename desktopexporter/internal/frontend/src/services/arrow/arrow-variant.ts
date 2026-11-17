import {
  readArrayLayout,
  readMapLayout,
  scalarView,
  type VariantMetadata,
  type VariantScalar,
} from './arrow-variant-codec'
import { cachedView, memoizeByIndex } from './lazy-view'

export { readVariantMetadata } from './arrow-variant-codec'
export type { VariantMetadata, VariantScalar } from './arrow-variant-codec'

export type VariantArray = {
  readonly kind: 'array'
  readonly childCount: number
  childAt(index: number): VariantValue
}

export type VariantMapEntry = {
  readonly key: string
  readonly value: VariantValue
}

export type VariantMap = {
  readonly kind: 'map'
  readonly childCount: number
  childAt(index: number): VariantMapEntry
}

export type VariantValue = VariantScalar | VariantArray | VariantMap

/** Read one complete OTel scalar. Retained bytes belong to the response. */
export function readVariantScalar(bytes: Uint8Array): VariantScalar {
  return cachedView(scalarView(bytes))
}

/** Open one value; each view owns its cache and shares the root's dictionary. */
export function readVariantValue(
  metadata: VariantMetadata,
  bytes: Uint8Array
): VariantValue {
  if (bytes.length > 0) {
    const basicType = bytes[0]! & 3
    if (basicType === 2) return mapView(metadata, bytes)
    if (basicType === 3) return arrayView(metadata, bytes)
  }
  return readVariantScalar(bytes)
}

function arrayView(metadata: VariantMetadata, bytes: Uint8Array): VariantArray {
  const layout = readArrayLayout(bytes)
  return {
    kind: 'array',
    childCount: layout.childCount,
    childAt: memoizeByIndex(index =>
      readVariantValue(metadata, layout.childBytes(index))
    ),
  }
}

function mapView(metadata: VariantMetadata, bytes: Uint8Array): VariantMap {
  const layout = readMapLayout(metadata, bytes)
  return {
    kind: 'map',
    childCount: layout.childCount,
    childAt: memoizeByIndex(index => {
      const field = layout.fieldAt(index)
      return cachedView({
        get key() {
          return metadata.keyAt(field.id)
        },
        get value() {
          return readVariantValue(metadata, field.bytes)
        },
      })
    }),
  }
}

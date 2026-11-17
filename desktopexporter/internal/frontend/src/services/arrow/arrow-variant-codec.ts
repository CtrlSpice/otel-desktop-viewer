// Parquet Variant encoding, carried by Arrow's arrow.parquet.variant extension:
// https://github.com/apache/parquet-format/blob/master/VariantEncoding.md

const utf8 = new TextDecoder('utf-8', { fatal: true, ignoreBOM: true })

export type VariantMetadata = {
  readonly keyCount: number
  keyBytes(index: number): Uint8Array
  keyAt(index: number): string
}

/** Read an unsigned little-endian integer without signed 32-bit coercion. */
function unsigned(bytes: Uint8Array, start: number, width: number): number {
  if (start + width > bytes.length)
    throw new Error('Invalid Variant encoding: truncated integer')
  let value = 0
  for (let i = width - 1; i >= 0; i--) value = value * 256 + bytes[start + i]!
  return value
}

function compareBytes(left: Uint8Array, right: Uint8Array): number {
  for (let i = 0; i < Math.min(left.length, right.length); i++) {
    const difference = left[i]! - right[i]!
    if (difference !== 0) return difference
  }
  return left.length - right.length
}

/** Validate dictionary bounds once; retain bytes and decode requested keys only. */
export function readVariantMetadata(bytes: Uint8Array): VariantMetadata {
  if (bytes.length === 0)
    throw new Error('Invalid Variant metadata: missing header')
  const header = bytes[0]!
  const version = header & 0x0f
  if (version !== 1) throw new Error(`Unsupported Variant version ${version}`)

  // Bits 6–7 store the offset width minus one. Bit 5 is reserved and ignored.
  const width = (header >>> 6) + 1
  const keyCount = unsigned(bytes, 1, width)
  const offsetsStart = 1 + width
  const stringsStart = offsetsStart + (keyCount + 1) * width
  if (stringsStart > bytes.length)
    throw new Error('Invalid Variant metadata: truncated offset table')
  const offsetAt = (index: number) =>
    unsigned(bytes, offsetsStart + index * width, width)
  if (offsetAt(0) !== 0)
    throw new Error('Invalid Variant metadata: first offset must be zero')

  let previous = 0
  let previousKey = bytes.subarray(stringsStart, stringsStart)
  for (let i = 0; i < keyCount; i++) {
    const end = offsetAt(i + 1)
    if (end < previous || end > bytes.length - stringsStart)
      throw new Error('Invalid Variant metadata: key offset out of range')
    const key = bytes.subarray(stringsStart + previous, stringsStart + end)
    if ((header & 0x10) !== 0 && i > 0 && compareBytes(previousKey, key) >= 0)
      throw new Error(
        'Invalid Variant metadata: dictionary is not sorted and unique'
      )
    previous = end
    previousKey = key
  }
  if (stringsStart + previous !== bytes.length)
    throw new Error('Invalid Variant metadata: trailing bytes')

  const keyBytes = (index: number): Uint8Array => {
    if (!Number.isInteger(index) || index < 0 || index >= keyCount)
      throw new RangeError(`Variant key ${index} is outside the dictionary`)
    return bytes.subarray(
      stringsStart + offsetAt(index),
      stringsStart + offsetAt(index + 1)
    )
  }
  return {
    keyCount,
    keyBytes,
    keyAt: index => utf8.decode(keyBytes(index)),
  }
}

export type VariantScalar =
  | { readonly kind: 'empty'; readonly value: null }
  | { readonly kind: 'bool'; readonly value: boolean }
  | { readonly kind: 'int64'; readonly value: bigint }
  | { readonly kind: 'double'; readonly value: number; readonly bits: bigint }
  | { readonly kind: 'string'; readonly value: string }
  | { readonly kind: 'bytes'; readonly value: Uint8Array }

function requireSize(bytes: Uint8Array, size: number): void {
  if (bytes.length !== size)
    throw new Error(
      `Invalid Variant value: expected ${size} bytes, received ${bytes.length}`
    )
}

/** Validate one OTel scalar; getters decode its retained bytes without caching. */
export function scalarView(bytes: Uint8Array): VariantScalar {
  if (bytes.length === 0)
    throw new Error('Invalid Variant value: missing header')
  const basicType = bytes[0]! & 3
  const header = bytes[0]! >>> 2
  if (basicType === 1) {
    // A short string stores its UTF-8 byte length in the header itself.
    requireSize(bytes, 1 + header)
    return {
      kind: 'string',
      get value() {
        return utf8.decode(bytes.subarray(1))
      },
    }
  }
  if (basicType !== 0)
    throw new Error('Expected a Variant scalar, received a container')

  // Respect a value's slice within the larger Arrow buffer.
  const data = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength)
  switch (header) {
    case 0: // null
      requireSize(bytes, 1)
      return { kind: 'empty', value: null }
    case 1: // true
    case 2: // false
      requireSize(bytes, 1)
      return { kind: 'bool', value: header === 1 }
    case 3: // int8
      requireSize(bytes, 2)
      return {
        kind: 'int64',
        get value() {
          return BigInt(data.getInt8(1))
        },
      }
    case 4: // int16
      requireSize(bytes, 3)
      return {
        kind: 'int64',
        get value() {
          return BigInt(data.getInt16(1, true))
        },
      }
    case 5: // int32
      requireSize(bytes, 5)
      return {
        kind: 'int64',
        get value() {
          return BigInt(data.getInt32(1, true))
        },
      }
    case 6: // int64
      requireSize(bytes, 9)
      return {
        kind: 'int64',
        get value() {
          return data.getBigInt64(1, true)
        },
      }
    case 7: // double
      requireSize(bytes, 9)
      return {
        kind: 'double',
        get value() {
          return data.getFloat64(1, true)
        },
        // Keep the received bits available even if JS arithmetic changes a NaN.
        get bits() {
          return data.getBigUint64(1, true)
        },
      }
    case 15: // binary
    case 16: {
      // string
      if (bytes.length < 5)
        throw new Error('Invalid Variant value: truncated byte length')
      requireSize(bytes, 5 + data.getUint32(1, true))
      return header === 15
        ? {
            kind: 'bytes',
            get value() {
              return bytes.subarray(5)
            },
          }
        : {
            kind: 'string',
            get value() {
              return utf8.decode(bytes.subarray(5))
            },
          }
    }
    default:
      throw new Error(`Unsupported Variant primitive type ${header}`)
  }
}

type VariantArrayLayout = {
  readonly childCount: number
  childBytes(index: number): Uint8Array
}

/** Validate this array's offsets; leave its child values unread. */
export function readArrayLayout(bytes: Uint8Array): VariantArrayLayout {
  const header = bytes[0]! >>> 2
  const width = (header & 3) + 1
  const countWidth = (header & 4) !== 0 ? 4 : 1
  if (bytes.length < 1 + countWidth)
    throw new Error('Invalid Variant array: truncated child count')
  const childCount = unsigned(bytes, 1, countWidth)
  const offsetsStart = 1 + countWidth
  const dataStart = offsetsStart + (childCount + 1) * width
  if (dataStart > bytes.length)
    throw new Error('Invalid Variant array: truncated offset table')
  const offsetAt = (index: number) =>
    unsigned(bytes, offsetsStart + index * width, width)
  if (offsetAt(0) !== 0)
    throw new Error('Invalid Variant array: first offset must be zero')

  let previous = 0
  for (let i = 0; i < childCount; i++) {
    const end = offsetAt(i + 1)
    // Even null has a one-byte header, so each child needs a nonempty range.
    if (end <= previous)
      throw new Error('Invalid Variant array: child offsets must increase')
    if (end > bytes.length - dataStart)
      throw new Error('Invalid Variant array: child offset out of range')
    previous = end
  }
  if (dataStart + previous !== bytes.length)
    throw new Error('Invalid Variant array: trailing bytes')

  return {
    childCount,
    childBytes(index) {
      if (!Number.isInteger(index) || index < 0 || index >= childCount)
        throw new RangeError(`Variant child ${index} is outside the array`)
      const start = dataStart + offsetAt(index)
      const end = dataStart + offsetAt(index + 1)
      return bytes.subarray(start, end)
    },
  }
}

type VariantMapLayout = {
  readonly childCount: number
  fieldAt(index: number): { readonly id: number; readonly bytes: Uint8Array }
}

/** Map fields are in key order; their value bytes may be in any order. */
export function readMapLayout(
  metadata: VariantMetadata,
  bytes: Uint8Array
): VariantMapLayout {
  const header = bytes[0]! >>> 2
  const width = (header & 3) + 1
  const idWidth = ((header >>> 2) & 3) + 1
  const countWidth = (header & 0x10) !== 0 ? 4 : 1
  if (bytes.length < 1 + countWidth)
    throw new Error('Invalid Variant map: truncated child count')
  const childCount = unsigned(bytes, 1, countWidth)
  const idsStart = 1 + countWidth
  const offsetsStart = idsStart + childCount * idWidth
  const dataStart = offsetsStart + (childCount + 1) * width
  if (dataStart > bytes.length)
    throw new Error('Invalid Variant map: truncated field tables')
  const idAt = (index: number) =>
    unsigned(bytes, idsStart + index * idWidth, idWidth)
  const offsetAt = (index: number) =>
    unsigned(bytes, offsetsStart + index * width, width)
  const dataSize = offsetAt(childCount)
  if (dataSize !== bytes.length - dataStart)
    throw new Error('Invalid Variant map: data size does not match byte length')
  if (childCount === 0 && dataSize !== 0)
    throw new Error('Invalid Variant map: trailing bytes')

  const physicalOrder = new Uint32Array(childCount)
  let previousKey = bytes.subarray(0, 0)
  for (let i = 0; i < childCount; i++) {
    const id = idAt(i)
    if (id >= metadata.keyCount)
      throw new Error('Invalid Variant map: key ID out of range')
    const key = metadata.keyBytes(id)
    if (i > 0 && compareBytes(previousKey, key) >= 0)
      throw new Error('Invalid Variant map: keys must be sorted and unique')
    previousKey = key
    if (offsetAt(i) >= dataSize)
      throw new Error('Invalid Variant map: child offset out of range')
    physicalOrder[i] = i
  }

  // Find each value's next physical boundary without decoding any values.
  physicalOrder.sort((left, right) => offsetAt(left) - offsetAt(right))
  const ends = new Uint32Array(childCount)
  for (let rank = 0; rank < childCount; rank++) {
    const index = physicalOrder[rank]!
    const start = offsetAt(index)
    const end =
      rank + 1 < childCount ? offsetAt(physicalOrder[rank + 1]!) : dataSize
    if (rank === 0 && start !== 0)
      throw new Error('Invalid Variant map: gap before first child')
    if (start === end)
      throw new Error('Invalid Variant map: duplicate child offset')
    ends[index] = end
  }

  return {
    childCount,
    fieldAt(index) {
      if (!Number.isInteger(index) || index < 0 || index >= childCount)
        throw new RangeError(`Variant child ${index} is outside the map`)
      const start = dataStart + offsetAt(index)
      const end = dataStart + ends[index]!
      return { id: idAt(index), bytes: bytes.subarray(start, end) }
    },
  }
}

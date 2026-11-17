import { describe, expect, it, vi } from 'vitest'
import {
  readVariantMetadata,
  readVariantScalar,
  readVariantValue,
} from './arrow-variant'

// Fixed format example: version 1, sorted keys, one-byte offsets, keys a and b.
const twoKeys = Uint8Array.of(0x11, 2, 0, 1, 2, 0x61, 0x62)

function dictionary(keys: string[], width: number, sorted = false): Uint8Array {
  const encoded = keys.map(key => new TextEncoder().encode(key))
  const size = encoded.reduce((sum, key) => sum + key.length, 0)
  const stringsStart = 1 + (keys.length + 2) * width
  const bytes = new Uint8Array(stringsStart + size)
  bytes[0] = 1 | (sorted ? 0x10 : 0) | ((width - 1) << 6)
  const write = (position: number, value: number) => {
    for (let i = 0; i < width; i++) {
      bytes[position + i] = value % 256
      value = Math.floor(value / 256)
    }
  }
  write(1, keys.length)
  let offset = 0
  encoded.forEach((key, index) => {
    write(1 + (index + 1) * width, offset)
    bytes.set(key, stringsStart + offset)
    offset += key.length
  })
  write(1 + (keys.length + 1) * width, offset)
  return bytes
}

describe('Variant metadata', () => {
  it('reads fixed reference bytes, including a view into a larger buffer', () => {
    const buffer = new Uint8Array(twoKeys.length + 12).fill(255)
    buffer.set(twoKeys, 5)
    const metadata = readVariantMetadata(buffer.subarray(5, 5 + twoKeys.length))
    expect(metadata.keyCount).toBe(2)
    expect(metadata.keyAt(0)).toBe('a')
    expect(metadata.keyAt(1)).toBe('b')
    expect(metadata.keyBytes(1).buffer).toBe(buffer.buffer)
  })

  it.each([1, 2, 3, 4])(
    'reads %i-byte offsets and preserves literal keys',
    width => {
      const keys = ['', '__proto__', 'a\0b', '雪', '\ufeffname']
      const metadata = readVariantMetadata(dictionary(keys, width))
      expect(metadata.keyCount).toBe(keys.length)
      expect(keys.map((_, index) => metadata.keyAt(index))).toEqual(keys)
    }
  )

  it('handles a dictionary whose offset table extends past 255 bytes', () => {
    const keys = Array.from({ length: 300 }, (_, index) => `key-${index}`)
    const metadata = readVariantMetadata(dictionary(keys, 2))
    expect(metadata.keyCount).toBe(300)
    expect(metadata.keyAt(299)).toBe('key-299')
  })

  it('accepts an empty dictionary and ignores the reserved header bit', () => {
    const empty = readVariantMetadata(Uint8Array.of(0x21, 0, 0))
    expect(empty.keyCount).toBe(0)
    expect(() => empty.keyAt(0)).toThrow('outside the dictionary')
  })

  it('compares sorted keys by UTF-8 bytes, not JavaScript UTF-16 order', () => {
    const keys = ['\ue000', '𐀀']
    expect([...keys].sort()).not.toEqual(keys)
    const metadata = readVariantMetadata(dictionary(keys, 1, true))
    expect(keys.map((_, index) => metadata.keyAt(index))).toEqual(keys)
  })

  it('allows unordered or repeated dictionary strings when the sorted flag is clear', () => {
    const metadata = readVariantMetadata(dictionary(['b', 'a', 'a'], 1))
    expect([0, 1, 2].map(index => metadata.keyAt(index))).toEqual([
      'b',
      'a',
      'a',
    ])
    expect(() => readVariantMetadata(dictionary(['b', 'a'], 1, true))).toThrow(
      'sorted and unique'
    )
    expect(() => readVariantMetadata(dictionary(['a', 'a'], 1, true))).toThrow(
      'sorted and unique'
    )
  })

  it.each([
    ['missing header', []],
    ['Unsupported Variant version 2', [2, 0, 0]],
    ['truncated integer', [0xc1, 0]],
    ['truncated offset table', [1, 2, 0]],
    ['truncated offset table', [0xc1, 0, 0, 0, 0x80]],
    ['first offset must be zero', [1, 1, 1, 2, 0x61, 0x62]],
    ['key offset out of range', [1, 2, 0, 2, 1, 0x61, 0x62]],
    ['key offset out of range', [1, 1, 0, 3, 0x61]],
    ['trailing bytes', [1, 0, 0, 0]],
    ['trailing bytes', [1, 1, 0, 1, 0x61, 0x62]],
  ] as const)('rejects malformed metadata: %s', (message, bytes) => {
    expect(() => readVariantMetadata(Uint8Array.from(bytes))).toThrow(message)
  })

  it.each([-1, 2, 0.5, NaN, Infinity])('rejects key index %s', index => {
    const metadata = readVariantMetadata(twoKeys)
    expect(() => metadata.keyAt(index)).toThrow('outside the dictionary')
  })

  it('rejects invalid UTF-8 when a key is requested instead of replacing its bytes', () => {
    const metadata = readVariantMetadata(Uint8Array.of(1, 1, 0, 1, 255))
    expect(() => metadata.keyAt(0)).toThrow()
  })
})

// Literal format vectors, rather than values encoded by the reader under test.
// Padding makes each scalar an unaligned view into a larger response buffer.
function scalarBytes(hex: string): Uint8Array {
  const buffer = new Uint8Array(hex.length / 2 + 8).fill(255)
  for (let i = 0; i < hex.length; i += 2)
    buffer[3 + i / 2] = parseInt(hex.slice(i, i + 2), 16)
  return buffer.subarray(3, 3 + hex.length / 2)
}

describe('Variant scalars', () => {
  it.each([
    ['00', { kind: 'empty', value: null }],
    ['04', { kind: 'bool', value: true }],
    ['08', { kind: 'bool', value: false }],
  ])('reads null and boolean header %s', (hex, expected) => {
    expect(readVariantScalar(scalarBytes(hex))).toEqual(expected)
  })

  it.each([
    ['0c00', 0n],
    ['0c01', 1n],
    ['0cff', -1n],
    ['0c80', -128n],
    ['0c7f', 127n],
    ['108000', 128n],
    ['107fff', -129n],
    ['100080', -32768n],
    ['10ff7f', 32767n],
    ['1400800000', 32768n],
    ['14ff7fffff', -32769n],
    ['1400000080', -2147483648n],
    ['14ffffff7f', 2147483647n],
    ['180000008000000000', 2147483648n],
    ['18ffffff7fffffffff', -2147483649n],
    ['180100000000002000', 9007199254740993n],
    ['18ffffffffffffff7f', 9223372036854775807n],
    ['180000000000000080', -9223372036854775808n],
  ])('reads signed integer %s exactly', (hex, expected) => {
    expect(readVariantScalar(scalarBytes(hex))).toEqual({
      kind: 'int64',
      value: expected,
    })
  })

  it.each(['0c01', '100100', '1401000000', '180100000000000000'])(
    'keeps integer one distinct from double one, with encoding %s',
    hex => {
      expect(readVariantScalar(scalarBytes(hex))).toEqual({
        kind: 'int64',
        value: 1n,
      })
      expect(readVariantScalar(scalarBytes('1c000000000000f03f'))).toEqual({
        kind: 'double',
        value: 1,
        bits: 0x3ff0000000000000n,
      })
    }
  )

  it.each([
    ['1c0000000000000000', 0x0000000000000000n, 0],
    ['1c0000000000000080', 0x8000000000000000n, -0],
    ['1c000000000000f03f', 0x3ff0000000000000n, 1],
    ['1c0100000000000000', 0x0000000000000001n, Number.MIN_VALUE],
    ['1cffffffffffffef7f', 0x7fefffffffffffffn, Number.MAX_VALUE],
    ['1c000000000000f07f', 0x7ff0000000000000n, Infinity],
    ['1c000000000000f0ff', 0xfff0000000000000n, -Infinity],
    ['1c420000000000f87f', 0x7ff8000000000042n, NaN],
    ['1c420000000000f8ff', 0xfff8000000000042n, NaN],
    ['1c420000000000f07f', 0x7ff0000000000042n, NaN],
  ])('retains double value and original bits %s', (hex, bits, expected) => {
    const value = readVariantScalar(scalarBytes(hex))
    expect(value.kind).toBe('double')
    if (value.kind !== 'double') throw new Error('Expected a double')
    expect(value.value).toBe(expected)
    expect(value.bits).toBe(bits)
  })

  it.each([
    ['01', ''],
    ['0d616263', 'abc'],
    ['0d610062', 'a\0b'],
    ['0defbbbf', '\ufeff'],
    ['1de99baaf0908080', '雪𐀀'],
    ['4000000000', ''],
    ['4003000000e99baa', '雪'],
    ['4003000000efbbbf', '\ufeff'],
  ])('reads UTF-8 string %s literally', (hex, expected) => {
    expect(readVariantScalar(scalarBytes(hex))).toEqual({
      kind: 'string',
      value: expected,
    })
  })

  it('reads strings at the short-string size boundary', () => {
    const short = '雪'.repeat(21) // 63 UTF-8 bytes, 21 JavaScript characters.
    const long = 'x'.repeat(64)
    expect(
      readVariantScalar(Uint8Array.of(0xfd, ...new TextEncoder().encode(short)))
        .value
    ).toBe(short)
    expect(
      readVariantScalar(
        Uint8Array.of(0x40, 64, 0, 0, 0, ...new TextEncoder().encode(long))
      ).value
    ).toBe(long)
  })

  it('decodes strings only when requested and caches the decoded text', () => {
    const decode = vi.spyOn(TextDecoder.prototype, 'decode')
    try {
      const value = readVariantScalar(scalarBytes('0d616263'))
      expect(value.kind).toBe('string')
      expect(decode).not.toHaveBeenCalled()
      expect(value.value).toBe('abc')
      expect(value.value).toBe('abc')
      expect(decode).toHaveBeenCalledTimes(1)
    } finally {
      decode.mockRestore()
    }
  })

  it.each(['05ff', '09c080', '0deda080', '4001000000ff'])(
    'rejects invalid UTF-8 when string %s is requested',
    hex => {
      const value = readVariantScalar(scalarBytes(hex))
      expect(value.kind).toBe('string')
      expect(() => value.value).toThrow()
    }
  )

  it.each([
    ['3c00000000', []],
    ['3c0300000000ff80', [0, 255, 128]],
  ])(
    'retains binary data %s without copying or decoding it',
    (hex, expected) => {
      const bytes = scalarBytes(hex)
      const value = readVariantScalar(bytes)
      expect(value.kind).toBe('bytes')
      if (value.kind !== 'bytes') throw new Error('Expected bytes')
      expect(value.value).toEqual(Uint8Array.from(expected))
      expect(value.value.buffer).toBe(bytes.buffer)
      expect(value.value.byteOffset).toBe(bytes.byteOffset + 5)
      expect(value.value).toBe(value.value)
    }
  )

  it.each([
    '00',
    '04',
    '08',
    '0c01',
    '100100',
    '1401000000',
    '180100000000002000',
    '1c420000000000f87f',
    '0d616263',
    '4003000000e99baa',
    '3c0300000000ff80',
  ])('rejects every truncation and extra bytes in scalar %s', hex => {
    const bytes = scalarBytes(hex)
    for (let end = 0; end < bytes.length; end++)
      expect(() => readVariantScalar(bytes.subarray(0, end))).toThrow()
    expect(() => readVariantScalar(Uint8Array.of(...bytes, 0))).toThrow(
      'Invalid Variant value: expected'
    )
  })

  it.each(['3c00000080', '40ffffffff'])(
    'rejects oversized unsigned byte length %s before reading the payload',
    hex => {
      expect(() => readVariantScalar(scalarBytes(hex))).toThrow(
        'Invalid Variant value: expected'
      )
    }
  )

  it.each([8, 9, 10, 11, 12, 13, 14, 17, 18, 19, 20, 63])(
    'rejects primitive type %i outside the OTel value types',
    type => {
      expect(() => readVariantScalar(Uint8Array.of(type << 2))).toThrow(
        `Unsupported Variant primitive type ${type}`
      )
    }
  )

  it.each([2, 3])('does not mistake container type %i for a scalar', type => {
    expect(() => readVariantScalar(Uint8Array.of(type, 0, 0))).toThrow(
      'Expected a Variant scalar'
    )
  })
})

const emptyMetadata = readVariantMetadata(Uint8Array.of(1, 0, 0))

function arrayBytes(
  children: Uint8Array[],
  width = 4,
  large = children.length > 255
): Uint8Array {
  const countWidth = large ? 4 : 1
  const dataStart = 1 + countWidth + (children.length + 1) * width
  const bytes = new Uint8Array(
    dataStart + children.reduce((sum, child) => sum + child.length, 0)
  )
  bytes[0] = 3 | ((width - 1) << 2) | (large ? 0x10 : 0)
  const write = (position: number, value: number, size: number) => {
    for (let i = 0; i < size; i++) {
      bytes[position + i] = value % 256
      value = Math.floor(value / 256)
    }
  }
  write(1, children.length, countWidth)
  let offset = 0
  children.forEach((child, index) => {
    write(1 + countWidth + index * width, offset, width)
    bytes.set(child, dataStart + offset)
    offset += child.length
  })
  write(1 + countWidth + children.length * width, offset, width)
  return bytes
}

describe('Variant arrays', () => {
  it('reads a fixed mixed array without changing order or scalar types', () => {
    // [1, "x", null, true, false, []], with offsets [0, 2, 4, 5, 6, 7, 10].
    const bytes = scalarBytes('03060002040506070a0c010578000408030000')
    const value = readVariantValue(emptyMetadata, bytes)
    expect(value.kind).toBe('array')
    if (value.kind !== 'array') throw new Error('Expected an array')
    expect(value.childCount).toBe(6)
    expect([0, 1, 2, 3, 4].map(index => value.childAt(index))).toEqual([
      { kind: 'int64', value: 1n },
      { kind: 'string', value: 'x' },
      { kind: 'empty', value: null },
      { kind: 'bool', value: true },
      { kind: 'bool', value: false },
    ])
    const empty = value.childAt(5)
    expect(empty.kind).toBe('array')
    if (empty.kind !== 'array') throw new Error('Expected an array')
    expect(empty.childCount).toBe(0)
    expect(() => empty.childAt(0)).toThrow('outside the array')
    expect(value.childAt(5)).toBe(empty)
  })

  it.each([
    '0301000104',
    '07010000010004',
    '0b0100000001000004',
    '0f01000000000100000004',
    '1301000000000104',
  ])('reads fixed offset/count encoding %s', hex => {
    const value = readVariantValue(emptyMetadata, scalarBytes(hex))
    if (value.kind !== 'array') throw new Error('Expected an array')
    expect(value.childCount).toBe(1)
    expect(value.childAt(0)).toEqual({ kind: 'bool', value: true })
  })

  it.each([1, 2, 3, 4])(
    'reads %i-byte offsets with either count width',
    width => {
      for (const large of [false, true]) {
        const value = readVariantValue(
          emptyMetadata,
          arrayBytes([scalarBytes('01'), scalarBytes('0c01')], width, large)
        )
        if (value.kind !== 'array') throw new Error('Expected an array')
        expect(value.childCount).toBe(2)
        expect(value.childAt(1)).toEqual({ kind: 'int64', value: 1n })
        expect(value.childAt(0)).toEqual({ kind: 'string', value: '' })
      }
    }
  )

  it.each([0x20, 0x40, 0x80, 0xe0])('ignores reserved header bits %i', bits => {
    const value = readVariantValue(
      emptyMetadata,
      Uint8Array.of(3 | bits, 1, 0, 1, 4)
    )
    if (value.kind !== 'array') throw new Error('Expected an array')
    expect(value.childCount).toBe(1)
    expect(value.childAt(0)).toEqual({ kind: 'bool', value: true })
  })

  it.each([255, 256, 300])('preserves all %i repeated array items', count => {
    const value = readVariantValue(
      emptyMetadata,
      arrayBytes(
        Array.from({ length: count }, () => scalarBytes('0c07')),
        2
      )
    )
    if (value.kind !== 'array') throw new Error('Expected an array')
    expect(value.childCount).toBe(count)
    for (let index = 0; index < count; index++)
      expect(value.childAt(index)).toEqual({ kind: 'int64', value: 7n })
  })

  it('reads the last child of a wide array without opening earlier children', () => {
    const count = 100000
    const children = Array<Uint8Array>(count).fill(Uint8Array.of(0xfc))
    children[count - 1] = scalarBytes('180100000000002000')
    const value = readVariantValue(emptyMetadata, arrayBytes(children))
    if (value.kind !== 'array') throw new Error('Expected an array')
    expect(value.childCount).toBe(count)
    const last = value.childAt(count - 1)
    expect(last).toEqual({ kind: 'int64', value: 9007199254740993n })
    expect(value.childAt(count - 1)).toBe(last)
    expect(() => value.childAt(0)).toThrow(
      'Unsupported Variant primitive type 63'
    )
  })

  it('leaves nested arrays unread until requested', () => {
    const value = readVariantValue(
      emptyMetadata,
      arrayBytes([Uint8Array.of(3), scalarBytes('0c01')])
    )
    if (value.kind !== 'array') throw new Error('Expected an array')
    expect(value.childCount).toBe(2)
    expect(value.childAt(1)).toEqual({ kind: 'int64', value: 1n })
    expect(() => value.childAt(0)).toThrow('truncated child count')
  })

  it('reads 200 nested arrays one level at a time', () => {
    let bytes = scalarBytes('180100000000002000')
    for (let level = 0; level < 200; level++)
      bytes = arrayBytes([scalarBytes('08'), bytes])
    let value = readVariantValue(emptyMetadata, bytes)
    for (let level = 0; level < 200; level++) {
      if (value.kind !== 'array') throw new Error('Expected an array')
      expect(value.childCount).toBe(2)
      expect(value.childAt(0)).toEqual({ kind: 'bool', value: false })
      value = value.childAt(1)
    }
    expect(value).toEqual({ kind: 'int64', value: 9007199254740993n })
  })

  it('retains binary child bytes in the original buffer', () => {
    const bytes = arrayBytes([scalarBytes('3c0200000000ff')])
    const value = readVariantValue(emptyMetadata, bytes)
    if (value.kind !== 'array') throw new Error('Expected an array')
    const child = value.childAt(0)
    if (child.kind !== 'bytes') throw new Error('Expected bytes')
    expect(child.value).toEqual(Uint8Array.of(0, 255))
    expect(child.value.buffer).toBe(bytes.buffer)
  })

  it.each([-1, 1, 0.5, NaN, Infinity])('rejects child index %s', index => {
    const value = readVariantValue(emptyMetadata, scalarBytes('0301000104'))
    if (value.kind !== 'array') throw new Error('Expected an array')
    expect(() => value.childAt(index)).toThrow('outside the array')
  })

  it.each([
    ['missing header', ''],
    ['truncated child count', '03'],
    ['truncated child count', '13010000'],
    ['truncated offset table', '030100'],
    ['truncated offset table', '13ffffffff'],
    ['truncated offset table', '1300000080'],
    ['first offset must be zero', '030101020000'],
    ['child offsets must increase', '03020002010c01'],
    ['child offsets must increase', '030200010100'],
    ['child offset out of range', '0301000200'],
    ['trailing bytes', '03000000'],
  ])('rejects malformed arrays: %s', (message, hex) => {
    expect(() => readVariantValue(emptyMetadata, scalarBytes(hex))).toThrow(
      message
    )
  })

  it('checks the exact byte range of a requested child', () => {
    // The offset table gives an int8 three bytes, but its encoding needs two.
    const value = readVariantValue(emptyMetadata, scalarBytes('030100030c0100'))
    if (value.kind !== 'array') throw new Error('Expected an array')
    expect(value.childCount).toBe(1)
    expect(() => value.childAt(0)).toThrow('expected 2 bytes, received 3')
  })

  it.each(['030000', '0301000104', '1301000000000104'])(
    'rejects every truncation and extra bytes in array %s',
    hex => {
      const bytes = scalarBytes(hex)
      for (let end = 0; end < bytes.length; end++)
        expect(() =>
          readVariantValue(emptyMetadata, bytes.subarray(0, end))
        ).toThrow()
      expect(() =>
        readVariantValue(emptyMetadata, Uint8Array.of(...bytes, 0))
      ).toThrow('trailing bytes')
    }
  )
})

function mapBytes(
  entries: { id: number; value: Uint8Array }[],
  {
    width = 4,
    idWidth = 4,
    large = entries.length > 255,
    physicalOrder = entries.map((_, index) => index),
  }: {
    width?: number
    idWidth?: number
    large?: boolean
    physicalOrder?: number[]
  } = {}
): Uint8Array {
  const countWidth = large ? 4 : 1
  const idsStart = 1 + countWidth
  const offsetsStart = idsStart + entries.length * idWidth
  const dataStart = offsetsStart + (entries.length + 1) * width
  const bytes = new Uint8Array(
    dataStart + entries.reduce((sum, entry) => sum + entry.value.length, 0)
  )
  bytes[0] = 2 | ((width - 1) << 2) | ((idWidth - 1) << 4) | (large ? 0x40 : 0)
  const write = (position: number, value: number, size: number) => {
    for (let i = 0; i < size; i++) {
      bytes[position + i] = value % 256
      value = Math.floor(value / 256)
    }
  }
  write(1, entries.length, countWidth)
  entries.forEach((entry, index) => {
    write(idsStart + index * idWidth, entry.id, idWidth)
  })
  let offset = 0
  for (const index of physicalOrder) {
    const entry = entries[index]!
    write(offsetsStart + index * width, offset, width)
    bytes.set(entry.value, dataStart + offset)
    offset += entry.value.length
  }
  write(offsetsStart + entries.length * width, offset, width)
  return bytes
}

describe('Variant maps', () => {
  it('reads fixed Go reference bytes whose key and physical value orders differ', () => {
    // The pinned Go Builder appends c=3, a=1, b=2. Field IDs are [1, 2, 0]
    // and offsets in key order are [2, 4, 0, 6]. Go also decodes these bytes.
    const metadata = readVariantMetadata(scalarBytes('010300010203636162'))
    const value = readVariantValue(
      metadata,
      scalarBytes('0203010200020400060c030c010c02')
    )
    expect(value.kind).toBe('map')
    if (value.kind !== 'map') throw new Error('Expected a map')
    expect(value.childCount).toBe(3)
    expect([0, 1, 2].map(index => value.childAt(index))).toEqual([
      { key: 'a', value: { kind: 'int64', value: 1n } },
      { key: 'b', value: { kind: 'int64', value: 2n } },
      { key: 'c', value: { kind: 'int64', value: 3n } },
    ])
  })

  it.each(['020000', '420000000000', '820000'])(
    'reads an empty map with encoding %s',
    hex => {
      const value = readVariantValue(emptyMetadata, scalarBytes(hex))
      if (value.kind !== 'map') throw new Error('Expected a map')
      expect(value.childCount).toBe(0)
      expect(() => value.childAt(0)).toThrow('outside the map')
    }
  )

  it.each([1, 2, 3, 4])(
    'reads %i-byte offsets with every field ID width and either count width',
    width => {
      const metadata = readVariantMetadata(dictionary(['z', 'a'], 1))
      for (const idWidth of [1, 2, 3, 4]) {
        for (const large of [false, true]) {
          const bytes = mapBytes(
            [
              { id: 1, value: scalarBytes('0c01') },
              { id: 0, value: scalarBytes('0c02') },
            ],
            { width, idWidth, large, physicalOrder: [1, 0] }
          )
          bytes[0]! |= 0x80 // The reserved bit must not change the layout.
          const value = readVariantValue(metadata, bytes)
          if (value.kind !== 'map') throw new Error('Expected a map')
          expect(value.childCount).toBe(2)
          expect(value.childAt(0)).toEqual({
            key: 'a',
            value: { kind: 'int64', value: 1n },
          })
          expect(value.childAt(1)).toEqual({
            key: 'z',
            value: { kind: 'int64', value: 2n },
          })
        }
      }
    }
  )

  it('uses literal keys in UTF-8 order, independently of dictionary ID order', () => {
    const keys = [
      '',
      '__proto__',
      'a\0b',
      'constructor',
      '雪',
      '\ue000',
      '\ufeffkey',
      '𐀀',
    ]
    const dictionaryKeys = [...keys].reverse()
    const metadata = readVariantMetadata(dictionary(dictionaryKeys, 1))
    const value = readVariantValue(
      metadata,
      mapBytes(
        keys.map((key, index) => ({
          id: dictionaryKeys.indexOf(key),
          value: Uint8Array.of(0x0c, index),
        }))
      )
    )
    if (value.kind !== 'map') throw new Error('Expected a map')
    expect(value.childCount).toBe(keys.length)
    keys.forEach((key, index) => {
      expect(value.childAt(index)).toEqual({
        key,
        value: { kind: 'int64', value: BigInt(index) },
      })
    })
  })

  it('allows unused duplicate dictionary strings but rejects duplicate map keys', () => {
    const metadata = readVariantMetadata(dictionary(['a', 'a', 'b'], 1))
    const field = (id: number) => ({ id, value: scalarBytes('00') })
    const value = readVariantValue(metadata, mapBytes([field(1), field(2)]))
    if (value.kind !== 'map') throw new Error('Expected a map')
    expect([value.childAt(0).key, value.childAt(1).key]).toEqual(['a', 'b'])
    expect(() =>
      readVariantValue(metadata, mapBytes([field(0), field(1)]))
    ).toThrow('keys must be sorted and unique')
  })

  it.each([255, 256, 300])(
    'reads all %i fields across dictionary and count boundaries',
    count => {
      const keys = Array.from(
        { length: count },
        (_, index) => `key-${String(index).padStart(3, '0')}`
      )
      const metadata = readVariantMetadata(dictionary(keys, 2, true))
      const value = readVariantValue(
        metadata,
        mapBytes(
          keys.map((_, id) => ({ id, value: scalarBytes('0c07') })),
          {
            width: 2,
            idWidth: 2,
            physicalOrder: keys.map((_, index) => count - 1 - index),
          }
        )
      )
      if (value.kind !== 'map') throw new Error('Expected a map')
      expect(value.childCount).toBe(count)
      keys.forEach((key, index) => {
        expect(value.childAt(index)).toEqual({
          key,
          value: { kind: 'int64', value: 7n },
        })
      })
    }
  )

  it('caches requested entries, keys and values without decoding other fields', () => {
    const metadata = readVariantMetadata(dictionary(['a', 'b', 'c'], 1))
    const keyAt = vi.spyOn(metadata, 'keyAt')
    try {
      const value = readVariantValue(
        metadata,
        mapBytes(
          [
            { id: 0, value: Uint8Array.of(3) },
            { id: 1, value: Uint8Array.of(0xfc) },
            { id: 2, value: scalarBytes('180100000000002000') },
          ],
          { physicalOrder: [1, 2, 0] }
        )
      )
      if (value.kind !== 'map') throw new Error('Expected a map')
      expect(value.childCount).toBe(3)
      const entry = value.childAt(2)
      expect(keyAt).not.toHaveBeenCalled()
      expect(entry.key).toBe('c')
      expect(entry.key).toBe('c')
      expect(keyAt).toHaveBeenCalledTimes(1)
      expect(entry.value).toEqual({ kind: 'int64', value: 9007199254740993n })
      expect(value.childAt(2)).toBe(entry)
      expect(entry.value).toBe(entry.value)
      expect(value.childAt(0).key).toBe('a')
      expect(() => value.childAt(0).value).toThrow('truncated child count')
      expect(() => value.childAt(1).value).toThrow(
        'Unsupported Variant primitive type 63'
      )
    } finally {
      keyAt.mockRestore()
    }
  })

  it('rejects invalid UTF-8 when a map key is requested', () => {
    const metadata = readVariantMetadata(Uint8Array.of(1, 1, 0, 1, 255))
    const value = readVariantValue(
      metadata,
      mapBytes([{ id: 0, value: scalarBytes('08') }])
    )
    if (value.kind !== 'map') throw new Error('Expected a map')
    const entry = value.childAt(0)
    expect(entry.value).toEqual({ kind: 'bool', value: false })
    expect(() => entry.key).toThrow()
  })

  it.each(['maps', 'mixed'])(
    'reads 200 levels of %s using the shared dictionary',
    nesting => {
      const metadata = readVariantMetadata(
        dictionary(['child', 'flag'], 1, true)
      )
      let bytes = scalarBytes('180100000000002000')
      for (let level = 199; level >= 0; level--) {
        bytes =
          nesting === 'maps' || level % 2 === 0
            ? mapBytes(
                [
                  { id: 0, value: bytes },
                  { id: 1, value: scalarBytes('08') },
                ],
                { physicalOrder: [1, 0] }
              )
            : arrayBytes([scalarBytes('08'), bytes])
      }
      let value = readVariantValue(metadata, bytes)
      for (let level = 0; level < 200; level++) {
        if (nesting === 'maps' || level % 2 === 0) {
          if (value.kind !== 'map') throw new Error('Expected a map')
          expect(value.childCount).toBe(2)
          expect(value.childAt(0).key).toBe('child')
          expect(value.childAt(1)).toEqual({
            key: 'flag',
            value: { kind: 'bool', value: false },
          })
          value = value.childAt(0).value
        } else {
          if (value.kind !== 'array') throw new Error('Expected an array')
          expect(value.childCount).toBe(2)
          expect(value.childAt(0)).toEqual({ kind: 'bool', value: false })
          value = value.childAt(1)
        }
      }
      expect(value).toEqual({ kind: 'int64', value: 9007199254740993n })
    }
  )

  it('retains binary buffers and double bits in map fields', () => {
    const metadata = readVariantMetadata(dictionary(['bytes', 'double'], 1))
    const bytes = mapBytes(
      [
        { id: 0, value: scalarBytes('3c0200000000ff') },
        { id: 1, value: scalarBytes('1c420000000000f87f') },
      ],
      { physicalOrder: [1, 0] }
    )
    const value = readVariantValue(metadata, bytes)
    if (value.kind !== 'map') throw new Error('Expected a map')
    const binary = value.childAt(0).value
    if (binary.kind !== 'bytes') throw new Error('Expected bytes')
    expect(binary.value).toEqual(Uint8Array.of(0, 255))
    expect(binary.value.buffer).toBe(bytes.buffer)
    const double = value.childAt(1).value
    if (double.kind !== 'double') throw new Error('Expected a double')
    expect(double.value).toBeNaN()
    expect(double.bits).toBe(0x7ff8000000000042n)
  })

  it.each([-1, 1, 0.5, NaN, Infinity])('rejects child index %s', index => {
    const value = readVariantValue(
      readVariantMetadata(twoKeys),
      scalarBytes('020100000100')
    )
    if (value.kind !== 'map') throw new Error('Expected a map')
    expect(() => value.childAt(index)).toThrow('outside the map')
  })

  it.each([1, 255, 256, 0x80000000, 0xffffffff])(
    'rejects invalid unsigned dictionary ID %i',
    id => {
      const metadata = readVariantMetadata(dictionary(['a'], 1))
      expect(() =>
        readVariantValue(metadata, mapBytes([{ id, value: scalarBytes('00') }]))
      ).toThrow('key ID out of range')
    }
  )

  it.each([
    ['truncated child count', '02'],
    ['truncated child count', '42010000'],
    ['truncated field tables', '020100'],
    ['truncated field tables', '42ffffffff'],
    ['truncated field tables', '4200000080'],
    ['data size does not match byte length', '020100000200'],
    ['data size does not match byte length', '02000000'],
    ['trailing bytes', '02000100'],
    ['key ID out of range', '020102000100'],
    ['keys must be sorted and unique', '020201000001020004'],
    ['keys must be sorted and unique', '020200000001020004'],
    ['child offset out of range', '020100010100'],
    ['child offset out of range', '020100020100'],
    ['gap before first child', '02010001020004'],
    ['duplicate child offset', '020200010000020004'],
  ])('rejects malformed maps: %s', (message, hex) => {
    expect(() =>
      readVariantValue(readVariantMetadata(twoKeys), scalarBytes(hex))
    ).toThrow(message)
  })

  it.each([
    ['gap between fields', '020200010003050c01000c02', 0],
    ['offset into another field', '020200010001040c010c02', 0],
    ['gap after last field', '020200010002050c010c0200', 1],
  ])('rejects %s when the affected value is requested', (_, hex, index) => {
    const value = readVariantValue(
      readVariantMetadata(twoKeys),
      scalarBytes(hex)
    )
    if (value.kind !== 'map') throw new Error('Expected a map')
    expect(() => value.childAt(index).value).toThrow(
      'Invalid Variant value: expected'
    )
  })

  it.each(['020000', '020100000100', '420100000000000100'])(
    'rejects every truncation and extra bytes in map %s',
    hex => {
      const metadata = readVariantMetadata(twoKeys)
      const bytes = scalarBytes(hex)
      // Check the reference itself, so malformed test input cannot satisfy the loop.
      const value = readVariantValue(metadata, bytes)
      if (value.kind !== 'map') throw new Error('Expected a map')
      for (let index = 0; index < value.childCount; index++)
        expect(value.childAt(index).value).toEqual({
          kind: 'empty',
          value: null,
        })
      for (let end = 0; end < bytes.length; end++)
        expect(() =>
          readVariantValue(metadata, bytes.subarray(0, end))
        ).toThrow()
      expect(() =>
        readVariantValue(metadata, Uint8Array.of(...bytes, 0))
      ).toThrow('data size does not match byte length')
    }
  )
})

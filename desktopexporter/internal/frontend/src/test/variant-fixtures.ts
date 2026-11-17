import {
  Binary,
  Field,
  RecordBatch,
  Schema,
  Struct,
  Table,
  makeData,
  vectorFromArray,
} from 'apache-arrow'
import type { AttributeFixture } from './telemetry-fixture-types'

export const variantStorageType = new Struct([
  new Field('metadata', new Binary(), false),
  new Field('value', new Binary(), false),
])

export function variantField(name: string, nullable = true): Field<Struct> {
  return new Field(
    name,
    variantStorageType,
    nullable,
    new Map([['ARROW:extension:name', 'arrow.parquet.variant']])
  )
}

/** Test writer: fixed-width tables keep this independent of the production reader. */
export function encodeVariant(value: AttributeFixture) {
  const utf8 = new TextEncoder()
  const names = new Set<string>()
  const pending = [value]
  while (pending.length) {
    const item = pending.pop()!
    if (item.kind === 'array') {
      for (const child of item.value) pending.push(child)
    } else if (item.kind === 'map') {
      for (const [key, child] of new Map(
        item.value.map(entry => [entry.key, entry.value])
      )) {
        names.add(key)
        pending.push(child)
      }
    }
  }
  const keys = [...names].map(key => ({ key, bytes: utf8.encode(key) }))
  keys.sort((a, b) => {
    for (let i = 0; i < Math.min(a.bytes.length, b.bytes.length); i++) {
      const difference = a.bytes[i]! - b.bytes[i]!
      if (difference) return difference
    }
    return a.bytes.length - b.bytes.length
  })
  const ids = new Map(keys.map(({ key }, index) => [key, index]))
  const stringsStart = 1 + (keys.length + 2) * 4
  const metadata = new Uint8Array(
    stringsStart + keys.reduce((sum, key) => sum + key.bytes.length, 0)
  )
  const dictionary = new DataView(metadata.buffer)
  metadata[0] = 0xd1 // version 1, sorted dictionary, four-byte offsets
  dictionary.setUint32(1, keys.length, true)
  let offset = 0
  keys.forEach(({ bytes }, index) => {
    dictionary.setUint32(5 + index * 4, offset, true)
    metadata.set(bytes, stringsStart + offset)
    offset += bytes.length
  })
  dictionary.setUint32(5 + keys.length * 4, offset, true)

  const encode = (item: AttributeFixture): Uint8Array => {
    switch (item.kind) {
      case 'empty':
        return Uint8Array.of(0)
      case 'bool':
        return Uint8Array.of(item.value ? 4 : 8)
      case 'int64': {
        const integer = BigInt(item.value)
        if (BigInt.asIntN(64, integer) !== integer)
          throw new Error('Fixture integer exceeds int64')
        const bytes = new Uint8Array(9)
        bytes[0] = 0x18
        new DataView(bytes.buffer).setBigInt64(1, integer, true)
        return bytes
      }
      case 'double': {
        const bytes = new Uint8Array(9)
        bytes[0] = 0x1c
        new DataView(bytes.buffer).setFloat64(1, item.value, true)
        return bytes
      }
      case 'string':
      case 'bytes': {
        const data =
          item.kind === 'string'
            ? utf8.encode(item.value)
            : Uint8Array.from(atob(item.value), character =>
                character.charCodeAt(0)
              )
        const short = item.kind === 'string' && data.length < 64
        const bytes = new Uint8Array((short ? 1 : 5) + data.length)
        bytes[0] = short
          ? (data.length << 2) | 1
          : item.kind === 'string'
            ? 0x40
            : 0x3c
        if (!short) new DataView(bytes.buffer).setUint32(1, data.length, true)
        bytes.set(data, short ? 1 : 5)
        return bytes
      }
      case 'array':
      case 'map': {
        const entries =
          item.kind === 'map'
            ? [
                ...new Map(item.value.map(entry => [entry.key, entry.value])),
              ].sort((a, b) => ids.get(a[0])! - ids.get(b[0])!)
            : undefined
        const children =
          item.kind === 'array'
            ? item.value.map(encode)
            : entries!.map(([, child]) => encode(child))
        const offsetsStart = 5 + (entries ? children.length * 4 : 0)
        const dataStart = offsetsStart + (children.length + 1) * 4
        const bytes = new Uint8Array(
          dataStart + children.reduce((sum, child) => sum + child.length, 0)
        )
        const data = new DataView(bytes.buffer)
        bytes[0] = entries ? 0x7e : 0x1f
        data.setUint32(1, children.length, true)
        entries?.forEach(([key], index) =>
          data.setUint32(5 + index * 4, ids.get(key)!, true)
        )
        let offset = 0
        children.forEach((child, index) => {
          data.setUint32(offsetsStart + index * 4, offset, true)
          bytes.set(child, dataStart + offset)
          offset += child.length
        })
        data.setUint32(offsetsStart + children.length * 4, offset, true)
        return bytes
      }
    }
  }
  return { metadata, value: encode(value) }
}

export function variantColumn(values: (AttributeFixture | null)[]) {
  const parts = values.map(value =>
    value === null ? null : encodeVariant(value)
  )
  const nullBitmap = new Uint8Array(Math.ceil(values.length / 8))
  values.forEach((value, index) => {
    if (value !== null) nullBitmap[index >> 3]! |= 1 << (index % 8)
  })
  return makeData({
    type: variantStorageType,
    length: values.length,
    nullCount: values.filter(value => value === null).length,
    nullBitmap,
    children: [
      vectorFromArray(
        parts.map(part => part?.metadata ?? null),
        new Binary()
      ).data[0]!,
      vectorFromArray(
        parts.map(part => part?.value ?? null),
        new Binary()
      ).data[0]!,
    ],
  })
}

export function variantTable(
  values: (AttributeFixture | null)[],
  name = 'value'
): Table {
  const field = variantField(name)
  const data = makeData({
    type: new Struct([field]),
    length: values.length,
    children: [variantColumn(values)],
  })
  return new Table(new RecordBatch(new Schema([field]), data))
}

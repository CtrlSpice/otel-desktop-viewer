import { describe, expect, it } from 'vitest'
import {
  attributeValueCopy,
  attributeValueTokens,
  hasConflictingKeys,
  jsonContainerTokens,
  compactValueTokens,
  valueTokenTree,
  type ValueNode,
} from './detail-value'
import type { AttributeValue } from '@/types/api-types'

describe('detail value formatting', () => {
  it('decodes bounded embedded-JSON previews without inventing their missing suffix', () => {
    const inner = '{"path":"/recorded/long/path…'
    const preview = `["loading", ${JSON.stringify(inner)}]`
    const tokens = compactValueTokens(preview, true)
    expect(tokens.map(token => token.text).join('')).toBe(
      `["loading", ${inner}]`
    )
    expect(tokens.find(token => token.kind === 'key')?.text).toBe('"path"')
    expect(
      tokens.filter(token => token.kind === 'string').map(token => token.text)
    ).toEqual(['"loading"', '"/recorded/long/path…'])
    expect(
      compactValueTokens(preview)
        .map(token => token.text)
        .join('')
    ).toBe(preview)
  })

  it('retains plain-string escapes, quoted keys and exact numbers in coloured previews', () => {
    const preview =
      '{"{\\"key\\":1}": "C:\\\\temp", "n": 9223372036854775807, "z": -0}'
    expect(
      compactValueTokens(preview, true)
        .map(token => token.text)
        .join('')
    ).toBe(preview)
  })

  it('formats JSON strings inside typed arrays and maps only for display', () => {
    const inner =
      '{"path":"/recorded/config.jsonc","n":9007199254740993,"n":-0}'
    const value: AttributeValue = {
      kind: 'array',
      value: [
        { kind: 'string', value: 'loading' },
        {
          kind: 'map',
          value: [{ key: 'payload', value: { kind: 'string', value: inner } }],
        },
        { kind: 'bytes', value: inner },
      ],
    }
    const display = attributeValueTokens(value, true)
      .map(token => token.text)
      .join('')
    expect(display).toContain(
      '"payload": {\n      "path": "/recorded/config.jsonc",\n      "n": 9007199254740993,\n      "n": -0\n    }'
    )
    expect(display).toContain(JSON.stringify(inner))
    expect(JSON.parse(attributeValueCopy(value))).toEqual([
      'loading',
      { payload: inner },
      inner,
    ])
  })

  it('formats embedded containers recursively without coercing keys, scalar strings or malformed JSON', () => {
    const inner = '{"n":1.2300e+40,"n":-0,"literal":"42"}'
    const nested = JSON.stringify({ payload: JSON.stringify([inner]) })
    const source = JSON.stringify([
      nested,
      'true',
      '42',
      '[invalid]',
      { '{"key":1}': '[]' },
    ])
    const display = jsonContainerTokens(source, true)!
      .map(token => token.text)
      .join('')
    expect(display).toContain(
      '"payload": [\n      {\n        "n": 1.2300e+40,\n        "n": -0,\n        "literal": "42"\n      }\n    ]'
    )
    expect(display).toContain('"true",\n  "42",\n  "[invalid]"')
    expect(display).toContain('"{\\"key\\":1}": []')
    expect(
      JSON.parse(
        jsonContainerTokens(source)!
          .map(token => token.text)
          .join('')
      )
    ).toEqual(JSON.parse(source))
  })

  it.each([
    '{message: "checkout: ready, now", paid: true, total: 49.95, … (+1 fields)}',
    '["line\\n\\\"quoted", object (3 fields), array (2 items), … (+2 items)]',
    '{sequence: 9223372036854775807, zero: -0, empty: null}',
    '{odd key: "<script>not HTML</script>", nested: object (2 fields)}',
  ])('colours compact previews without changing their text: %s', text => {
    expect(
      compactValueTokens(text)
        .map(token => token.text)
        .join('')
    ).toBe(text)
  })

  it('recognizes compact keys, strings, exact numbers and literals', () => {
    const tokens = compactValueTokens(
      '{message: "a:b", paid: true, total: 9223372036854775807}'
    )
    expect(
      tokens.filter(token => token.kind === 'key').map(token => token.text)
    ).toEqual(['message', 'paid', 'total'])
    expect(tokens.find(token => token.kind === 'string')?.text).toBe('"a:b"')
    expect(tokens.find(token => token.kind === 'literal')?.text).toBe('true')
    expect(tokens.find(token => token.kind === 'number')?.text).toBe(
      '9223372036854775807'
    )
  })

  it('groups nested tokens for folding without losing duplicate keys or numeric lexemes', () => {
    const tokens = jsonContainerTokens(
      '{"same":9007199254740993,"same":-0,"items":[{}, {"a":1.2300e+40},false]}'
    )!
    const tree = valueTokenTree(tokens, 'payload')
    function flatten(nodes: ValueNode[]): string {
      return nodes
        .map(node =>
          node.kind === 'container'
            ? node.opening.text + flatten(node.children) + node.closing.text
            : node.text
        )
        .join('')
    }
    expect(flatten(tree)).toBe(tokens.map(token => token.text).join(''))
    const root = tree[0]!
    expect(root.kind).toBe('container')
    if (root.kind !== 'container') throw new Error('Expected root container')
    expect(root).toMatchObject({ path: 'payload', count: 3 })
    const items = root.children.find(node => node.kind === 'container')!
    expect(items).toMatchObject({ path: 'payload.items', count: 3 })
    if (items.kind !== 'container') throw new Error('Expected array container')
    expect(
      items.children.filter(node => node.kind === 'container')
    ).toMatchObject([
      { path: 'payload.items[0]', count: 0 },
      { path: 'payload.items[1]', count: 1 },
    ])
  })

  it('indents JSON source without changing order, duplicate keys, numeric lexemes or escapes', () => {
    const source = String.raw` {"z":9007199254740993,"z":-0,"a":[1.2300e+40,{},[],"x\n\"y"],"ok":true} `
    const tokens = jsonContainerTokens(source)!
    expect(tokens.map(token => token.text).join('')).toBe(
      '{\n  "z": 9007199254740993,\n  "z": -0,\n  "a": [\n    1.2300e+40,\n    {},\n    [],\n    "x\\n\\\"y"\n  ],\n  "ok": true\n}'
    )
    expect(
      tokens.filter(token => token.kind === 'key').map(token => token.text)
    ).toEqual(['"z"', '"z"', '"a"', '"ok"'])
  })

  it.each([
    '[not json]',
    '{"x":1,}',
    '{"x":1} trailing',
    '42',
    'true',
    '"text"',
    'heatmap-playground',
  ])('leaves non-container text untouched: %s', text => {
    expect(jsonContainerTokens(text)).toBeNull()
  })

  it('copies raw strings and bytes without presentation quotes or JSON coercion', () => {
    for (const kind of ['string', 'bytes'] as const) {
      expect(
        attributeValueCopy({ kind, value: '{"value":9007199254740993}' })
      ).toBe('{"value":9007199254740993}')
    }
    expect(
      attributeValueTokens({ kind: 'string', value: '{"a":1}' })
        .map(token => token.text)
        .join('')
    ).toBe('"{\\"a\\":1}"')
  })
  it('preserves int64 bounds without a Number conversion', () => {
    for (const value of [
      -9_223_372_036_854_775_808n,
      9_223_372_036_854_775_807n,
    ]) {
      expect(attributeValueCopy({ kind: 'int64', value })).toBe(String(value))
    }
  })
  it('outputs valid indented JSON for arrays, maps, booleans, bytes and null', () => {
    const value: AttributeValue = {
      kind: 'map',
      value: [
        {
          key: 'quoted"key',
          value: {
            kind: 'array',
            value: [
              { kind: 'bool', value: false },
              { kind: 'empty', value: null },
              { kind: 'bytes', value: 'AQI=' },
            ],
          },
        },
      ],
    }
    expect(attributeValueCopy(value)).toBe(
      JSON.stringify({ 'quoted"key': [false, null, 'AQI='] }, null, 2)
    )
    expect(JSON.parse(attributeValueCopy(value))).toEqual({
      'quoted"key': [false, null, 'AQI='],
    })
  })
  it('preserves empty containers, signed zero and non-finite double spellings', () => {
    expect(attributeValueCopy({ kind: 'map', value: [] })).toBe('{}')
    expect(attributeValueCopy({ kind: 'array', value: [] })).toBe('[]')
    expect(attributeValueCopy({ kind: 'double', value: -0 })).toBe('-0')
    for (const value of [NaN, Infinity, -Infinity]) {
      expect(attributeValueCopy({ kind: 'double', value })).toBe(
        JSON.stringify(String(value))
      )
    }
  })
  it('detects conflicts through nested arrays without merging the duplicate entries', () => {
    const value: AttributeValue = {
      kind: 'array',
      value: [
        {
          kind: 'map',
          conflictingKeys: ['same'],
          value: [
            { key: 'same', value: { kind: 'int64', value: 1n } },
            { key: 'same', value: { kind: 'string', value: '1' } },
          ],
        },
      ],
    }
    expect(hasConflictingKeys(value)).toBe(true)
    expect(attributeValueCopy(value)).toContain('"same": 1,\n    "same": "1"')
    expect(hasConflictingKeys({ kind: 'map', value: [] })).toBe(false)
  })
})

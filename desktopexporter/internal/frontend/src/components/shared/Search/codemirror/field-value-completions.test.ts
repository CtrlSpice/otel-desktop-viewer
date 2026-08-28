import { describe, expect, it, vi } from 'vitest'
import { EditorState } from '@codemirror/state'
import { CompletionContext } from '@codemirror/autocomplete'
import { createFieldValueSource } from './field-value-completions'
import { createFieldValueCache } from './field-value-cache'
import { getFieldsBySignal } from '@/constants/fields'
import { queryLanguage } from './query-language'

const NAMES = ['checkout/pay', 'checkout/verify', 'db select "users"']

function makeSource(
  signal = 'traces',
  fetch: (
    signal: string,
    field: string,
    term: string,
    limit: number
  ) => Promise<string[]> = async () => NAMES
) {
  return {
    source: createFieldValueSource(createFieldValueCache(fetch, signal), () =>
      getFieldsBySignal(signal as 'traces' | 'logs' | 'metrics')
    ),
    fetch,
  }
}

async function run(
  doc: string,
  signal = 'traces',
  fetch?: () => Promise<string[]>
) {
  const { source } = makeSource(signal, fetch)
  const state = EditorState.create({ doc, extensions: [queryLanguage] })
  return source(new CompletionContext(state, doc.length, false))
}

describe('span name value completions', () => {
  it('offers the fetched names in the empty value position', async () => {
    const r = await run('name = ')
    expect(r).not.toBeNull()
    expect(r!.options.map(o => o.label)).toEqual(NAMES)
    expect(r!.from).toBe('name = '.length)
  })

  it('anchors at the value start while a name is being typed', async () => {
    const r = await run('name = che')
    expect(r).not.toBeNull()
    expect(r!.from).toBe('name = '.length)
  })

  it('anchors after an opening quote so filtering sees the bare name', async () => {
    // Anchoring ON the quote made CodeMirror filter labels against `"che`,
    // which no label matches -- the popup came up empty in the live editor.
    const r = await run('name = "che')
    expect(r).not.toBeNull()
    expect(r!.from).toBe('name = "'.length)
    // The user's quote is kept; the apply closes it.
    expect(r!.options[0].apply).toBe('checkout/pay"')
  })

  it('does not double a closing quote closeBrackets already inserted', async () => {
    // Typing " in the real editor auto-inserts the pair, leaving the cursor
    // inside: name = "che|". The apply must not add a second closing quote.
    const doc = 'name = "che"'
    const { source } = makeSource()
    const state = EditorState.create({ doc, extensions: [queryLanguage] })
    const r = await source(
      new CompletionContext(state, 'name = "che'.length, false)
    )
    expect(r).not.toBeNull()
    expect(r!.options[0].apply).toBe('checkout/pay')
  })

  it('escapes quotes inside a name when applying', async () => {
    const r = await run('name = ')
    const db = r!.options.find(o => o.label === 'db select "users"')
    expect(db?.apply).toBe('"db select \\"users\\""')
  })

  it('keeps a single-quote opening and escapes accordingly', async () => {
    const r = await run("name = 'db")
    expect(r).not.toBeNull()
    const db = r!.options.find(o => o.label === 'db select "users"')
    // Inside single quotes the double quotes need no escape.
    expect(db?.apply).toBe('db select "users"\'')
  })

  it('stays out of non-discoverable fields and free text', async () => {
    expect(await run('kind = ')).toBeNull()
    expect(await run('statusMessage = ')).toBeNull()
    expect(await run('che')).toBeNull()
  })

  it('serves any discoverable field, per signal', async () => {
    const fetch = vi.fn(async () => ['ms', 's'])
    const { source } = makeSource('metrics', fetch)
    const state = EditorState.create({
      doc: 'unit = ',
      extensions: [queryLanguage],
    })
    const r = await source(new CompletionContext(state, 7, false))
    expect(r).not.toBeNull()
    expect(fetch).toHaveBeenCalledWith('metrics', 'unit', '', 500)
  })

  it('fetches each field once per session, shared across triggers', async () => {
    const fetch = vi.fn(async () => NAMES)
    const { source } = makeSource('traces', fetch)
    for (const doc of ['name = ', 'name = c', 'name = ch']) {
      const state = EditorState.create({ doc, extensions: [queryLanguage] })
      await source(new CompletionContext(state, doc.length, false))
    }
    expect(fetch).toHaveBeenCalledTimes(1)
  })

  it('stays out once the value is closed', async () => {
    expect(await run('name = "a" ')).toBeNull()
  })

  it('fetches with an empty term and the session limit', async () => {
    const fetch = vi.fn(async () => NAMES)
    const { source } = makeSource('traces', fetch)
    const state = EditorState.create({
      doc: 'name = ',
      extensions: [queryLanguage],
    })
    await source(new CompletionContext(state, 7, false))
    expect(fetch).toHaveBeenCalledWith('traces', 'name', '', 500)
  })

  it('returns nothing when the fetch fails', async () => {
    expect(
      await run('name = ', 'traces', async () => {
        throw new Error('store down')
      })
    ).toBeNull()
  })
})

describe('IN array values', () => {
  // Metric `unit` is the field that actually offers IN among the
  // discoverable ones; span name deliberately does not.
  const UNITS = ['ms', 's', 'By']

  async function units(doc: string, pos = doc.length) {
    const source = createFieldValueSource(
      createFieldValueCache(async () => UNITS, 'metrics'),
      () => getFieldsBySignal('metrics')
    )
    const state = EditorState.create({ doc, extensions: [queryLanguage] })
    return source(new CompletionContext(state, pos, false))
  }

  it('offers values for the first item in an array', async () => {
    const r = await units('unit IN [')
    expect(r).not.toBeNull()
    expect(r!.from).toBe('unit IN ['.length)
    expect(r!.options.map(o => o.label)).toEqual(UNITS)
  })

  it('anchors at the item being typed, not the bracket', async () => {
    // Anchoring at the bracket made accepting replace the whole array with
    // one bare value, destroying the items already listed.
    const r = await units('unit IN ["ms", s')
    expect(r).not.toBeNull()
    expect(r!.from).toBe('unit IN ["ms", '.length)
  })

  it('anchors after an opening quote on a later item', async () => {
    const r = await units('unit IN ["ms", "B')
    expect(r).not.toBeNull()
    expect(r!.from).toBe('unit IN ["ms", "'.length)
    expect(r!.options[0].apply).toBe('ms"')
  })

  it('declines once an item is closed but the array is not', async () => {
    // After `"ms"` only a comma or a bracket is valid, not another value.
    expect(await units('unit IN ["ms"')).toBeNull()
  })

  it('declines once the array is closed', async () => {
    expect(await units('unit IN ["ms"] ')).toBeNull()
  })
})

describe('operator gating', () => {
  async function traces(doc: string) {
    const source = createFieldValueSource(
      createFieldValueCache(async () => NAMES, 'traces'),
      () => getFieldsBySignal('traces')
    )
    const state = EditorState.create({ doc, extensions: [queryLanguage] })
    return source(new CompletionContext(state, doc.length, false))
  }

  async function metrics(doc: string) {
    const source = createFieldValueSource(
      createFieldValueCache(async () => ['ms', 's'], 'metrics'),
      () => getFieldsBySignal('metrics')
    )
    const state = EditorState.create({ doc, extensions: [queryLanguage] })
    return source(new CompletionContext(state, doc.length, false))
  }

  it('declines an operator the field does not accept', async () => {
    // span name takes no array operator, so completing inside one would help
    // write exactly what the linter underlines.
    expect(await traces('name IN [')).toBeNull()
    expect(await traces('name IN ')).toBeNull()
  })

  it('offers the bracket after an array operator', async () => {
    // Bare values there complete into `unit IN ms`, which does not parse.
    const r = await metrics('unit IN ')
    expect(r).not.toBeNull()
    expect(r!.options.map(o => o.label)).toEqual(['['])
    // A function rather than a string: it reopens completion on the values
    // inside, so the bracket is one step rather than a dead end.
    expect(typeof r!.options[0].apply).toBe('function')
  })

  it('offers values once the bracket is there', async () => {
    const r = await metrics('unit IN [')
    expect(r).not.toBeNull()
    expect(r!.options.map(o => o.label)).toEqual(['ms', 's'])
  })

  it('still completes scalar operators normally', async () => {
    const r = await metrics('unit = ')
    expect(r).not.toBeNull()
    expect(r!.options.map(o => o.label)).toEqual(['ms', 's'])
  })
})

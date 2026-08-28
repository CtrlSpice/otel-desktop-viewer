import { describe, expect, it, vi } from 'vitest'
import { EditorState } from '@codemirror/state'
import { CompletionContext } from '@codemirror/autocomplete'
import { createSpanNameValueSource } from './span-name-completions'
import { queryLanguage } from './query-language'

const NAMES = ['checkout/pay', 'checkout/verify', 'db select "users"']

function makeSource(
  signal = 'traces',
  fetch: (term: string, limit: number) => Promise<string[]> = async () => NAMES
) {
  return { source: createSpanNameValueSource(fetch, signal), fetch }
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

  it('replaces an opening quote so the applied value is quoted once', async () => {
    const r = await run('name = "che')
    expect(r).not.toBeNull()
    expect(r!.from).toBe('name = '.length)
    const apply = r!.options[0].apply
    expect(apply).toBe('"checkout/pay"')
  })

  it('escapes quotes inside a name when applying', async () => {
    const r = await run('name = ')
    const db = r!.options.find(o => o.label === 'db select "users"')
    expect(db?.apply).toBe('"db select \\"users\\""')
  })

  it('stays out of other fields, other signals, and free text', async () => {
    expect(await run('kind = ')).toBeNull()
    expect(await run('name = ', 'metrics')).toBeNull()
    expect(await run('che')).toBeNull()
  })

  it('stays out once the value is closed', async () => {
    expect(await run('name = "a" ')).toBeNull()
  })

  it('fetches once with an empty term and the session limit', async () => {
    const fetch = vi.fn(async () => NAMES)
    const { source } = makeSource('traces', fetch)
    const state = EditorState.create({
      doc: 'name = ',
      extensions: [queryLanguage],
    })
    await source(new CompletionContext(state, 7, false))
    expect(fetch).toHaveBeenCalledWith('', 500)
  })

  it('returns nothing when the fetch fails', async () => {
    expect(
      await run('name = ', 'traces', async () => {
        throw new Error('store down')
      })
    ).toBeNull()
  })
})

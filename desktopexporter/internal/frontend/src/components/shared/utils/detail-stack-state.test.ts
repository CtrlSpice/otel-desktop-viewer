// @vitest-environment jsdom
import { beforeEach, describe, expect, it } from 'vitest'
import { createDetailStackState } from './detail-stack-state.svelte'

function fixture(
  available = [true, true, true],
  labels = ['Span', 'Events', 'Links']
) {
  const stack = createDetailStackState('test-detail-handoff')
  const sections = labels.map((label, index) => {
    let open = available[index]!
    const section = {
      id: label,
      label,
      element: document.createElement('div'),
      available: () => available[index]!,
      expanded: () => available[index]! && open,
      setExpanded: (value: boolean) => {
        open = value
      },
    }
    stack.register(section)
    return section
  })
  return {
    stack,
    sections,
    open: () =>
      sections.flatMap(section => (section.expanded() ? [section.label] : [])),
  }
}

beforeEach(() => localStorage.clear())

describe('detail heading collapse handoff', () => {
  it.each([
    { Span: 1e308 },
    { Span: 1e308, Events: 1e308, Links: 1e308 },
    { Span: 1e308, Events: Number.MIN_VALUE, Links: 1 },
  ])(
    'restores overflow-sized preferences without invalid geometry: %o',
    saved => {
      localStorage.setItem('test-detail-handoff', JSON.stringify(saved))
      const { stack } = fixture()
      const sizes = () =>
        ['Span', 'Events', 'Links'].map(name => parseFloat(stack.flex(name)))
      expect(sizes().every(size => Number.isFinite(size) && size >= 0)).toBe(
        true
      )
      expect(sizes().reduce((sum, size) => sum + size, 0)).toBeCloseTo(100)
      stack.setOpen('Events', false)
      stack.setOpen('Events', true)
      expect(sizes().every(size => Number.isFinite(size) && size >= 0)).toBe(
        true
      )
      expect(sizes().reduce((sum, size) => sum + size, 0)).toBeCloseTo(100)
    }
  )

  it('keeps valid fractional saved sizing unchanged', () => {
    localStorage.setItem(
      'test-detail-handoff',
      '{"Span":0.6,"Events":0.3,"Links":0.1}'
    )
    const { stack } = fixture()
    expect(parseFloat(stack.flex('Span'))).toBeCloseTo(60)
    expect(parseFloat(stack.flex('Events'))).toBeCloseTo(30)
    expect(parseFloat(stack.flex('Links'))).toBeCloseTo(10)
  })

  it('treats prototype-shaped section names as literal sizing keys', () => {
    const names = ['constructor', 'toString', '__proto__']
    const { stack } = fixture([true, true, true], names)
    for (const name of names)
      expect(parseFloat(stack.flex(name))).toBeCloseTo(100 / 3)
    stack.setOpen('__proto__', false)
    expect(parseFloat(stack.flex('toString'))).toBeCloseTo(200 / 3)
    stack.setOpen('__proto__', true)
    for (const name of names)
      expect(Number.isFinite(parseFloat(stack.flex(name)))).toBe(true)
    expect(
      Object.keys(JSON.parse(localStorage.getItem('test-detail-handoff')!))
    ).toEqual(names)
    const restored = fixture([true, true, true], names).stack
    for (const name of names) expect(restored.flex(name)).toBe(stack.flex(name))
  })
  it('skips unavailable headers and restores their resize position when data arrives', () => {
    const available = [false, false, true]
    const { stack } = fixture(available)
    expect(stack.previous('Links')).toBeUndefined()
    available[0] = true
    stack.ensureOpen()
    expect(stack.previous('Links')?.id).toBe('Span')
    available[1] = true
    expect(stack.previous('Links')?.id).toBe('Span')
    stack.setOpen('Events', true)
    expect(stack.previous('Links')?.id).toBe('Events')
    available[1] = false
    expect(stack.previous('Links')?.id).toBe('Span')
  })

  it.each([
    ['Events', 'Span', 'Links'],
    ['Links', 'Events', 'Span'],
  ])(
    'hands %s space to %s without resizing %s',
    (source, receiver, untouched) => {
      const { stack, open } = fixture()
      const before = stack.flex(untouched)
      stack.setOpen(source, false)
      expect(open()).not.toContain(source)
      expect(open()).toContain(receiver)
      expect(stack.flex(untouched)).toBe(before)
      expect(parseFloat(stack.flex(receiver))).toBeCloseTo(200 / 3)
    }
  )

  it('keeps the primary section open even when other sections are open', () => {
    const { stack, sections, open } = fixture()
    expect(stack.canClose('Span')).toBe(false)
    stack.setOpen('Span', false)
    expect(open()).toEqual(['Span', 'Events', 'Links'])
    sections[1]!.setExpanded(false)
    sections[2]!.setExpanded(false)
    stack.setOpen('Span', false)
    expect(open()).toEqual(['Span'])
    expect(parseFloat(stack.flex('Span'))).toBeCloseTo(100)
  })

  it('hands space upward past empty neighbours', () => {
    const { stack, open } = fixture([true, false, true])
    stack.setOpen('Links', false)
    expect(open()).toEqual(['Span'])
  })

  it('keeps the sole non-empty section open', () => {
    const { stack, open } = fixture([true, false, false])
    expect(stack.canClose('Span')).toBe(false)
    stack.setOpen('Span', false)
    expect(open()).toEqual(['Span'])
    expect(parseFloat(stack.flex('Span'))).toBe(100)
  })
})

// @vitest-environment jsdom
import { beforeEach, describe, expect, it } from 'vitest'
import { createDetailStackState } from './detail-stack-state.svelte'

function fixture(available = [true, true, true]) {
  const stack = createDetailStackState('test-detail-handoff')
  const sections = ['Span', 'Events', 'Links'].map((label, index) => {
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

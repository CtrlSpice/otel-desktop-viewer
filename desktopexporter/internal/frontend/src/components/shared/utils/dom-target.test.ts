// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  isElementTarget,
  isHTMLElementTarget,
  isNodeTarget,
} from './dom-target'

afterEach(() => {
  document.body.innerHTML = ''
})

describe('DOM target realm checks', () => {
  it('uses the candidate node own realm constructors', () => {
    const iframe = document.createElement('iframe')
    document.body.appendChild(iframe)
    const foreignDocument = iframe.contentDocument
    if (!foreignDocument) throw new Error('Expected iframe document')
    const element = foreignDocument.createElement('div')
    const text = foreignDocument.createTextNode('text')
    const svg = foreignDocument.createElementNS(
      'http://www.w3.org/2000/svg',
      'svg'
    )

    expect(element).not.toBeInstanceOf(Node)
    expect(foreignDocument).not.toBeInstanceOf(Node)
    expect(isNodeTarget(foreignDocument)).toBe(true)
    expect(isNodeTarget(element)).toBe(true)
    expect(isNodeTarget(text)).toBe(true)
    expect(isElementTarget(element)).toBe(true)
    expect(isElementTarget(svg)).toBe(true)
    expect(isHTMLElementTarget(element)).toBe(true)
    expect(isHTMLElementTarget(svg)).toBe(false)
  })

  it('rejects a structurally convincing EventTarget without throwing', () => {
    const spoof = new EventTarget()
    Object.defineProperties(spoof, {
      nodeType: { value: 1 },
      nodeName: { value: 'DIV' },
      ownerDocument: { value: document },
      closest: { value: vi.fn() },
      contains: { value: vi.fn() },
    })

    expect(() => isNodeTarget(spoof)).not.toThrow()
    expect(isNodeTarget(spoof)).toBe(false)
    expect(isElementTarget(spoof)).toBe(false)
    expect(isHTMLElementTarget(spoof)).toBe(false)
  })

  it('rejects native nodes whose document has no live realm', () => {
    const detachedDocument = document.implementation.createHTMLDocument()
    const detachedElement = detachedDocument.createElement('div')

    expect(detachedDocument.defaultView).toBeNull()
    expect(() => isNodeTarget(detachedDocument)).not.toThrow()
    expect(isNodeTarget(detachedDocument)).toBe(false)
    expect(isNodeTarget(detachedElement)).toBe(false)
    expect(isElementTarget(detachedElement)).toBe(false)
    expect(isHTMLElementTarget(detachedElement)).toBe(false)
  })
})

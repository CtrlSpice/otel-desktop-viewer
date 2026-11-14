import { ChevronRightIcon } from '@hugeicons/core-free-icons'

export const JSON_FOLD_CARET = ChevronRightIcon

/** CodeMirror owns gutter DOM; use the same Hugeicons data as the Svelte renderer. */
export function createJsonFoldCaret(): SVGSVGElement {
  const namespace = 'http://www.w3.org/2000/svg'
  const svg = document.createElementNS(namespace, 'svg')
  svg.setAttribute('viewBox', '0 0 24 24')
  svg.setAttribute('fill', 'none')
  svg.setAttribute('aria-hidden', 'true')
  svg.setAttribute('focusable', 'false')
  svg.classList.add('json-fold-icon')
  for (const [tag, attributes] of JSON_FOLD_CARET) {
    const element = document.createElementNS(namespace, tag)
    for (const [name, value] of Object.entries(attributes)) {
      if (name !== 'key')
        element.setAttribute(
          name.replace(/[A-Z]/g, letter => `-${letter.toLowerCase()}`),
          String(value)
        )
    }
    svg.append(element)
  }
  return svg
}

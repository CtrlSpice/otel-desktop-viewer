import type { SyntaxNode } from '@lezer/common'

/** Resolve a JSON Pointer over original source, using the last duplicate key. */
export function evalSourceNode(
  payload: string,
  pointer: string,
  root: SyntaxNode | null
): SyntaxNode {
  let node = root
  for (const segment of pointer.split('/').slice(1)) {
    const key = segment.replace(/~1/g, '/').replace(/~0/g, '~')
    let next: SyntaxNode | null = null
    if (node?.name === 'Object') {
      for (const property of node.getChildren('Property')) {
        const name = property.getChild('PropertyName')!
        if (JSON.parse(payload.slice(name.from, name.to)) === key)
          next = property.lastChild
      }
    } else if (node?.name === 'Array') {
      let index = 0
      for (let child = node.firstChild; child; child = child.nextSibling) {
        if (['[', ']', ','].includes(child.name)) continue
        if (String(index++) === key) {
          next = child
          break
        }
      }
    }
    node = next
    if (!node) throw new Error('Source field is missing.')
  }
  if (!node) throw new Error('Source field is missing.')
  return node
}

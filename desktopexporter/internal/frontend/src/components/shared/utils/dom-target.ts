const ELEMENT_NODE = 1

export function isNodeTarget(target: EventTarget | null): target is Node {
  return (
    target !== null &&
    'nodeType' in target &&
    'nodeName' in target &&
    'ownerDocument' in target
  )
}

export function isElementTarget(target: EventTarget | null): target is Element {
  return (
    isNodeTarget(target) &&
    target.nodeType === ELEMENT_NODE &&
    'closest' in target &&
    'contains' in target
  )
}

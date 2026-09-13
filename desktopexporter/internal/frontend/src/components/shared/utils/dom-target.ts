function hasNativeNodeBrand(target: EventTarget): target is Node {
  try {
    Node.prototype.getRootNode.call(target)
    return true
  } catch {
    return false
  }
}

function hasNativeDocumentBrand(target: Node): target is Document {
  try {
    Document.prototype.hasFocus.call(target)
    return true
  } catch {
    return false
  }
}

export function isNodeTarget(target: EventTarget | null): target is Node {
  if (target === null || !hasNativeNodeBrand(target)) return false
  const ownerDocument = target.ownerDocument
  if (ownerDocument === null) {
    if (!hasNativeDocumentBrand(target)) return false
    const NodeConstructor = target.defaultView?.Node
    return NodeConstructor !== undefined && target instanceof NodeConstructor
  }
  const NodeConstructor = ownerDocument.defaultView?.Node
  return NodeConstructor !== undefined && target instanceof NodeConstructor
}

export function isElementTarget(target: EventTarget | null): target is Element {
  if (!isNodeTarget(target)) return false
  const ElementConstructor = target.ownerDocument?.defaultView?.Element
  return (
    ElementConstructor !== undefined && target instanceof ElementConstructor
  )
}

export function isHTMLElementTarget(
  target: EventTarget | null
): target is HTMLElement {
  if (!isElementTarget(target)) return false
  const HTMLElementConstructor = target.ownerDocument.defaultView?.HTMLElement
  return (
    HTMLElementConstructor !== undefined &&
    target instanceof HTMLElementConstructor
  )
}

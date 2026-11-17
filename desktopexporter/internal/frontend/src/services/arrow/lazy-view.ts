/** Response-owned lazy values; consumers must derive rather than mutate arrays. */
export function memoize<T>(read: () => T): () => T {
  let initialized = false
  let value: T
  return () => {
    if (!initialized) {
      value = read()
      if (Array.isArray(value)) Object.freeze(value)
      initialized = true
    }
    return value
  }
}

/** Retain requested children only. A failed read leaves the index uncached. */
export function memoizeByIndex<T>(
  read: (index: number) => T
): (index: number) => T {
  const values = new Map<number, T>()
  return index => {
    if (!values.has(index)) values.set(index, read(index))
    return values.get(index)!
  }
}

/** Cache a response view's getters without evaluating them. */
export function cachedView<T extends object>(view: T): T {
  const descriptors = Object.getOwnPropertyDescriptors(view)
  for (const descriptor of Object.values(descriptors)) {
    if (descriptor.get) descriptor.get = memoize(descriptor.get.bind(view))
  }
  return Object.defineProperties(view, descriptors)
}

/** Compose views without evaluating getters as object spread would. */
export function extendView<T extends object, U extends object>(
  base: T,
  extra: U
): T & U {
  // SAFETY: All T descriptors are installed on the U object without reading them.
  return Object.defineProperties(
    extra,
    Object.getOwnPropertyDescriptors(base)
  ) as T & U
}

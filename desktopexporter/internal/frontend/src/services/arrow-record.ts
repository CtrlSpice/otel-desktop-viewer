import { ArrowCell, ArrowFrame } from './arrow-data'

/** Define enumerable column getters without decoding the row's values. */
export function columnView<T extends object>(
  frame: ArrowFrame,
  row: number,
  readers: { [K in keyof T]: (cell: ArrowCell) => T[K] },
  cache = false
): T {
  const descriptors: PropertyDescriptorMap = {}
  for (const name in readers) {
    descriptors[name] = {
      enumerable: true,
      get: cache
        ? memoize(() => readers[name](frame.cell(name, row)))
        : () => readers[name](frame.cell(name, row)),
    }
  }
  // SAFETY: The mapped readers contract requires a decoder for every T field;
  // each descriptor returns that field's declared type.
  return Object.defineProperties({}, descriptors) as T
}

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

/** Cache a response view's getters without visiting its retained Arrow columns. */
export function cachedView<T extends object>(view: T): T {
  const descriptors = Object.getOwnPropertyDescriptors(view)
  for (const descriptor of Object.values(descriptors)) {
    if (descriptor.get) descriptor.get = memoize(descriptor.get.bind(view))
  }
  return Object.defineProperties(view, descriptors)
}

export function cachedColumnView<T extends object>(
  frame: ArrowFrame,
  row: number,
  readers: { [K in keyof T]: (cell: ArrowCell) => T[K] }
): T {
  return columnView(frame, row, readers, true)
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

export const text = (cell: ArrowCell): string =>
  cell.isNull ? '' : cell.text()
export const integer = (cell: ArrowCell): bigint => cell.integer()
export const number = (cell: ArrowCell): number => cell.number()
export const boolean = (cell: ArrowCell): boolean => cell.boolean()
export const nullableInteger = (cell: ArrowCell): bigint | null =>
  cell.isNull ? null : cell.integer()
export const nullableNumber = (cell: ArrowCell): number | null =>
  cell.isNull ? null : cell.number()
export const nullableText = (cell: ArrowCell): string | null =>
  cell.isNull ? null : cell.text()

export function listValues<T>(
  cell: ArrowCell,
  read: (cell: ArrowCell) => T
): T[] {
  const values = cell.list()
  return Array.from({ length: values.length }, (_, row) =>
    read(new ArrowCell(values, row))
  )
}

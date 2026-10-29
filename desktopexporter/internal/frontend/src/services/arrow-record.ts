import { ArrowCell, ArrowFrame } from './arrow-data'

/** Define enumerable column getters without decoding the row's values. */
export function columnView<T extends object>(
  frame: ArrowFrame,
  row: number,
  readers: { [K in keyof T]: (cell: ArrowCell) => T[K] }
): T {
  const descriptors: PropertyDescriptorMap = {}
  for (const name in readers) {
    descriptors[name] = {
      enumerable: true,
      get: () => readers[name](frame.cell(name, row)),
    }
  }
  // SAFETY: The mapped readers contract requires a decoder for every T field;
  // each descriptor returns that field's declared type.
  return Object.defineProperties({}, descriptors) as T
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

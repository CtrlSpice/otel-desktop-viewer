import { DataType, Table, Vector, makeVector, tableFromIPC } from 'apache-arrow'
import type { Field } from 'apache-arrow'

export const arrowStreamContentType = 'application/vnd.apache.arrow.stream'
const noFieldMetadata: ReadonlyMap<string, string> = new Map()

/** A retained Arrow dataset. Views keep columns and row locations, not row copies. */
export class ArrowFrame {
  private readonly resolvedColumns = new Map<
    string,
    { vector: Vector; field: Field }
  >()
  constructor(readonly columns: Table | Vector) {
    // A branch returning only [] has Arrow List<Null>, with no row fields to
    // infer. It is a valid empty dataset; nonempty values still require Struct.
    if (
      !(columns instanceof Table) &&
      !DataType.isStruct(columns.type) &&
      !(columns.length === 0 && DataType.isNull(columns.type))
    )
      throw new Error(`Expected Arrow row columns, received ${columns.type}`)
  }

  get length(): number {
    return this.columns instanceof Table
      ? this.columns.numRows
      : this.columns.length
  }

  column(name: string): Vector {
    return this.columnEntry(name).vector
  }

  private columnEntry(name: string) {
    const resolved = this.resolvedColumns.get(name)
    if (resolved) return resolved
    const vector = this.columns.getChild(name)
    const fields: readonly Field[] =
      this.columns instanceof Table
        ? this.columns.schema.fields
        : this.columns.type.children
    const field = fields.find(field => field.name === name)
    if (!vector || !field)
      throw new Error(`Arrow response is missing column ${name}`)
    const entry = { vector, field }
    this.resolvedColumns.set(name, entry)
    return entry
  }

  has(name: string): boolean {
    return this.columns.getChild(name) !== null
  }

  cell(name: string, row: number): ArrowCell {
    const { vector, field } = this.columnEntry(name)
    return new ArrowCell(vector, row, field.metadata)
  }

  /** Resolve the column once when supplying a chart/table accessor. */
  accessor(name: string): (row: number) => ArrowCell {
    const { vector, field } = this.columnEntry(name)
    return row => new ArrowCell(vector, row, field.metadata)
  }
}

/** One typed location. Nested values are opened only when a consumer asks. */
export class ArrowCell {
  readonly vector: Vector
  readonly row: number
  readonly fieldMetadata: ReadonlyMap<string, string>

  constructor(vector: Vector, row: number, fieldMetadata = noFieldMetadata) {
    if (!Number.isInteger(row) || row < 0 || row >= vector.length)
      throw new RangeError(`Arrow row ${row} is outside the vector`)
    this.fieldMetadata = fieldMetadata
    if (!DataType.isUnion(vector.type) && !DataType.isDictionary(vector.type)) {
      this.vector = vector
      this.row = row
      return
    }
    // Slicing retains the buffers and resolves chunk boundaries. It does not
    // turn nested values into JavaScript arrays or objects.
    const data = vector.slice(row, row + 1).data[0]!
    if (DataType.isUnion(data.type)) {
      const child = data.type.typeIdToChildIndex[data.typeIds[0]!]!
      const index = DataType.isDenseUnion(data.type) ? data.valueOffsets[0]! : 0
      const resolved = new ArrowCell(
        makeVector(data.children[child]!),
        index,
        data.type.children[child]!.metadata
      )
      this.vector = resolved.vector
      this.row = resolved.row
      this.fieldMetadata = resolved.fieldMetadata
    } else if (DataType.isDictionary(data.type) && vector.isValid(row)) {
      const resolved = new ArrowCell(
        data.dictionary!,
        Number(data.values[0]),
        fieldMetadata
      )
      this.vector = resolved.vector
      this.row = resolved.row
      this.fieldMetadata = resolved.fieldMetadata
    } else {
      this.vector = vector
      this.row = row
    }
  }

  get type(): DataType {
    return this.vector.type
  }

  get extensionName(): string | undefined {
    return this.fieldMetadata.get('ARROW:extension:name')
  }

  get isNull(): boolean {
    return DataType.isNull(this.type) || !this.vector.isValid(this.row)
  }

  text(): string {
    if (this.isNull || !DataType.isUtf8(this.type))
      throw new Error(`Expected Arrow Utf8, received ${this.type}`)
    return this.vector.get(this.row)
  }

  boolean(): boolean {
    if (this.isNull || !DataType.isBool(this.type))
      throw new Error(`Expected Arrow Bool, received ${this.type}`)
    return this.vector.get(this.row)
  }

  integer(): bigint {
    if (this.isNull) throw new Error('Expected an integer, received Arrow null')
    if (DataType.isInt(this.type)) return BigInt(this.vector.get(this.row))
    // DuckDB uses Decimal128(scale=0) for HUGEINT arithmetic. Do not route
    // durations, deltas or unsigned histogram sums through JS Number.
    if (DataType.isDecimal(this.type) && this.type.scale === 0) {
      const words: Uint32Array = this.vector.get(this.row)
      let result = 0n
      for (let i = words.length - 1; i >= 0; i--)
        result = (result << 32n) | BigInt(words[i]!)
      return BigInt.asIntN(words.length * 32, result)
    }
    throw new Error(`Expected an Arrow integer, received ${this.type}`)
  }

  number(): number {
    if (this.isNull) throw new Error('Expected a number, received Arrow null')
    if (DataType.isFloat(this.type)) return this.vector.get(this.row)
    const value = this.integer()
    const converted = Number(value)
    if (!Number.isSafeInteger(converted))
      throw new Error('Arrow integer exceeds the exact JavaScript number range')
    return converted
  }

  bytes(): Uint8Array {
    if (
      this.isNull ||
      !(DataType.isBinary(this.type) || DataType.isFixedSizeBinary(this.type))
    )
      throw new Error(`Expected Arrow Binary, received ${this.type}`)
    return this.vector.get(this.row)
  }

  list(): Vector {
    if (this.isNull || !DataType.isList(this.type))
      throw new Error(`Expected Arrow List, received ${this.type}`)
    return this.vector.get(this.row)
  }

  struct(): ArrowFrame {
    if (this.isNull || !DataType.isStruct(this.type))
      throw new Error(`Expected Arrow Struct, received ${this.type}`)
    return new ArrowFrame(
      makeVector(this.vector.slice(this.row, this.row + 1).data[0]!)
    )
  }

  map(): ArrowFrame {
    if (this.isNull || !DataType.isMap(this.type))
      throw new Error(`Expected Arrow Map, received ${this.type}`)
    const data = this.vector.slice(this.row, this.row + 1).data[0]!
    const start = Number(data.valueOffsets[0])
    const end = Number(data.valueOffsets[1])
    // Expose entry columns rather than MapRow's object-property proxy. Literal
    // keys such as __proto__ and constructor stay ordinary received map keys.
    return new ArrowFrame(
      makeVector(data.children[0]!.slice(start, end - start))
    )
  }
}

/** Reject a truncated stream, including one ending exactly between batches. */
export function decodeArrow(
  bytes: Uint8Array,
  expectedRows: bigint
): ArrowFrame {
  const end = [255, 255, 255, 255, 0, 0, 0, 0]
  if (
    bytes.length < end.length ||
    end.some((byte, i) => bytes[bytes.length - end.length + i] !== byte)
  )
    throw new Error('Incomplete Arrow response')
  const table = tableFromIPC(bytes)
  if (BigInt(table.numRows) !== expectedRows)
    throw new Error('Arrow response row count does not match its header')
  return new ArrowFrame(table)
}

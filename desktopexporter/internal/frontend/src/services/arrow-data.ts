import { DataType, Table, Vector, makeVector, tableFromIPC } from 'apache-arrow'

export const arrowStreamContentType = 'application/vnd.apache.arrow.stream'

/** A retained Arrow dataset. Views keep columns and row locations, not row copies. */
export class ArrowFrame {
  constructor(readonly columns: Table | Vector) {
    if (!(columns instanceof Table) && !DataType.isStruct(columns.type))
      throw new Error(`Expected Arrow row columns, received ${columns.type}`)
  }

  get length(): number {
    return this.columns instanceof Table
      ? this.columns.numRows
      : this.columns.length
  }

  column(name: string): Vector {
    const column = this.columns.getChild(name)
    if (!column) throw new Error(`Arrow response is missing column ${name}`)
    return column
  }

  cell(name: string, row: number): ArrowCell {
    return new ArrowCell(this.column(name), row)
  }

  /** Resolve the column once when supplying a chart/table accessor. */
  accessor(name: string): (row: number) => ArrowCell {
    const column = this.column(name)
    return row => new ArrowCell(column, row)
  }
}

/** One typed location. Nested values are opened only when a consumer asks. */
export class ArrowCell {
  readonly vector: Vector
  readonly row: number

  constructor(vector: Vector, row: number) {
    if (!Number.isInteger(row) || row < 0 || row >= vector.length)
      throw new RangeError(`Arrow row ${row} is outside the vector`)
    // Slicing retains the buffers and resolves chunk boundaries. It does not
    // turn nested values into JavaScript arrays or objects.
    const data = vector.slice(row, row + 1).data[0]!
    if (DataType.isUnion(data.type)) {
      const child = data.type.typeIdToChildIndex[data.typeIds[0]!]!
      const index = DataType.isDenseUnion(data.type) ? data.valueOffsets[0]! : 0
      const resolved = new ArrowCell(makeVector(data.children[child]!), index)
      this.vector = resolved.vector
      this.row = resolved.row
    } else if (DataType.isDictionary(data.type) && vector.isValid(row)) {
      const resolved = new ArrowCell(data.dictionary!, Number(data.values[0]))
      this.vector = resolved.vector
      this.row = resolved.row
    } else {
      this.vector = vector
      this.row = row
    }
  }

  get type(): DataType {
    return this.vector.type
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

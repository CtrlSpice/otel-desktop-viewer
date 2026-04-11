export type FixedColumn = {
  kind: 'fixed'
  width: number
}

export type ResizableColumn = {
  kind: 'resizable'
  min: number
  default: number
}

export type ElasticColumn = {
  kind: 'elastic'
  min: number
}

export type ColumnSizing = FixedColumn | ResizableColumn | ElasticColumn

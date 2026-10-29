import type { AttributeValue, MetricSeriesViewData } from '@/types/api-types'
import type { LegendTimeseries } from '@/types/metric-chart-types'
import { attributeValueLabel } from '@/components/shared/attribute-label'

type Column = {
  key: string
  signatures: string[]
  labels: string[]
  cost: number
}
type Partition = number[][]
export type SeriesLabel = { text: string; needsIdentity: boolean }
export type SeriesCell = {
  label: string
  preview?: string
  kind?: string
  signature: string
  values: readonly AttributeValue[]
}
export type SeriesColumn = { key: string; cells: Map<string, SeriesCell> }
export type SeriesTable = {
  columns: SeriesColumn[]
  labels: Map<string, SeriesLabel>
}

function valueSignature(value: AttributeValue): string {
  // Preserve bigint and non-finite/-0 doubles without delimiter ambiguities in
  // arrays, maps or strings. This is a display comparison, never a series key.
  switch (value.kind) {
    case 'double':
      return JSON.stringify([
        value.kind,
        Object.is(value.value, -0) ? '-0' : String(value.value),
      ])
    case 'int64':
      return JSON.stringify([value.kind, String(value.value)])
    case 'array':
      return JSON.stringify([value.kind, value.value.map(valueSignature)])
    case 'map':
      return JSON.stringify([
        value.kind,
        value.value.map(entry => [entry.key, valueSignature(entry.value)]),
      ])
    default:
      return JSON.stringify([value.kind, value.value])
  }
}

function split(groups: Partition, column: Column): Partition {
  return groups.flatMap(group => {
    const buckets = new Map<string, number[]>()
    for (const row of group) {
      const value = column.signatures[row]!
      const bucket = buckets.get(value)
      if (bucket) bucket.push(row)
      else buckets.set(value, [row])
    }
    return [...buckets.values()]
  })
}

function minimumColumns(columns: Column[], rowCount: number): Column[] {
  const initial = [Array.from({ length: rowCount }, (_, index) => index)]
  const target = columns.reduce(split, initial).length
  if (target <= 1) return []

  // A deterministic greedy cover preserves every distinction even when proving
  // a minimum would be too expensive for the UI thread.
  function greedy(): Column[] {
    const chosen: Column[] = []
    let groups = initial
    while (groups.length < target) {
      let best: Column | undefined
      let bestGroups = groups
      for (const column of columns) {
        if (chosen.includes(column)) continue
        const refined = split(groups, column)
        if (refined.length > bestGroups.length) {
          best = column
          bestGroups = refined
        }
      }
      if (!best) break
      chosen.push(best)
      groups = bestGroups
    }
    return chosen
  }
  if (columns.length > 16) return greedy()

  // A key is mandatory when removing it merges otherwise distinct rows.
  // This also avoids combinatorial searches for independent binary labels.
  const required = columns.filter(
    (_, index) =>
      columns.filter((_, other) => index !== other).reduce(split, initial)
        .length < target
  )
  const remaining = columns.filter(column => !required.includes(column))
  const base = required.reduce(split, initial)
  let remainingWork = 100_000
  const refine = (groups: Partition, column: Column) => {
    remainingWork -= rowCount
    return split(groups, column)
  }

  function search(
    start: number,
    slots: number,
    groups: Partition
  ): Column[] | undefined {
    if (groups.length === target) return []
    if (remainingWork <= 0 || slots === 0 || remaining.length - start < slots)
      return undefined
    // No subset of the remaining keys can do better than all of them together.
    if (remaining.slice(start).reduce(refine, groups).length < target)
      return undefined
    for (let index = start; index <= remaining.length - slots; index++) {
      if (remainingWork <= 0) return undefined
      const column = remaining[index]!
      const refined = refine(groups, column)
      if (refined.length === groups.length) continue
      const suffix = search(index + 1, slots - 1, refined)
      if (suffix) return [column, ...suffix]
    }
    return undefined
  }

  // Increasing cardinality finds an exact minimum within the work budget.
  // Columns are already ordered by display length, then key, for stable ties.
  for (let size = 0; size <= remaining.length; size++) {
    const chosen = search(0, size, base)
    if (chosen) return [...required, ...chosen]
    if (remainingWork <= 0) return greedy()
  }
  return columns
}

/** A compact distinguishing key set for all rows, including hidden series. */
export function minimalSeriesTable(
  rows: readonly LegendTimeseries[],
  preselected = false
): SeriesTable {
  if (rows.length === 0) return { columns: [], labels: new Map() }
  const attributes = rows.map(row => {
    const byKey = new Map<string, AttributeValue[]>()
    for (const { key, value } of row.attributes) {
      const values = byKey.get(key)
      if (values) values.push(value)
      else byKey.set(key, [value])
    }
    return byKey
  })
  const keys = [
    ...new Set(rows.flatMap(row => row.attributes.map(attr => attr.key))),
  ]
  const columns: Column[] = keys
    .flatMap(key => {
      const values = attributes.map(row => row.get(key) ?? [])
      const signatures = values.map(items =>
        JSON.stringify(items.map(valueSignature).sort())
      )
      const labels = values.map(items =>
        items.length === 0
          ? '[absent]'
          : items.map(attributeValueLabel).sort().join(', ')
      )
      const meanings = new Map<string, Set<string>>()
      labels.forEach((label, index) => {
        const known = meanings.get(label) ?? new Set<string>()
        known.add(signatures[index]!)
        meanings.set(label, known)
      })
      const displays = labels.map((label, index) => {
        if (meanings.get(label)!.size === 1) return label
        // A string "1" and a numeric 1 must not become identical visible labels.
        // Structural collisions likewise keep their complete typed representation.
        const items = values[index]!
        const typed = `${label} (${
          items
            .map(item => item.kind)
            .sort()
            .join(', ') || 'missing'
        })`
        const sameTypeCollision = values.some(
          (other, otherIndex) =>
            labels[otherIndex] === label &&
            signatures[otherIndex] !== signatures[index] &&
            other
              .map(item => item.kind)
              .sort()
              .join(', ') ===
              items
                .map(item => item.kind)
                .sort()
                .join(', ')
        )
        return sameTypeCollision ? signatures[index]! : typed
      })
      const column = {
        key,
        signatures,
        labels: displays,
        cost: displays.reduce(
          (sum, label) => sum + key.length + label.length,
          0
        ),
      }
      return new Set(signatures).size > 1 ? [column] : []
    })
    .sort(
      (a, b) => a.cost - b.cost || (a.key < b.key ? -1 : a.key > b.key ? 1 : 0)
    )

  // Equivalent columns distinguish exactly the same groups. Retain the shorter
  // one (e.g. model rather than session.id when both identify the same sessions).
  const partitions = new Set<string>()
  const candidates = columns.filter(column => {
    const groups = new Map<string, number>()
    const signature = column.signatures
      .map(value => {
        if (!groups.has(value)) groups.set(value, groups.size)
        return groups.get(value)
      })
      .join(',')
    if (partitions.has(signature)) return false
    partitions.add(signature)
    return true
  })
  const selected = (
    preselected ? columns : minimumColumns(candidates, rows.length)
  ).sort((a, b) => (a.key < b.key ? -1 : a.key > b.key ? 1 : 0))
  const texts = rows.map(
    (_, index) =>
      selected
        .map(column => `${column.key}: ${column.labels[index]}`)
        .join(' · ') || 'default series'
  )
  const counts = new Map<string, number>()
  for (const text of texts) counts.set(text, (counts.get(text) ?? 0) + 1)
  const labels = new Map(
    rows.map((row, index) => [
      row.key,
      {
        text: texts[index]!,
        needsIdentity: counts.get(texts[index]!)! > 1,
      },
    ])
  )
  return {
    labels,
    columns: selected.map(column => ({
      key: column.key,
      cells: new Map(
        rows.map((row, index) => [
          row.key,
          {
            label: column.labels[index]!,
            signature: column.signatures[index]!,
            values: attributes[index]!.get(column.key) ?? [],
          },
        ])
      ),
    })),
  }
}

export function minimalSeriesLabels(
  rows: readonly LegendTimeseries[]
): Map<string, SeriesLabel> {
  return minimalSeriesTable(rows).labels
}

/** Render server-selected previews without opening exact sort values. */
export function compactSeriesTable(
  series: readonly Pick<MetricSeriesViewData, 'seriesRef' | 'labelPreviews'>[]
): SeriesTable {
  const previews = series.map(
    row => new Map(row.labelPreviews?.map(cell => [cell.key, cell]))
  )
  const keys = [...new Set(previews.flatMap(row => [...row.keys()]))].sort()
  const texts = series.map(
    (_, index) =>
      keys
        .map(key => `${key}: ${previews[index]!.get(key)?.text ?? '[absent]'}`)
        .join(' · ') || 'default series'
  )
  const counts = new Map<string, number>()
  for (const text of texts) counts.set(text, (counts.get(text) ?? 0) + 1)
  return {
    labels: new Map(
      series.map((row, index) => [
        row.seriesRef,
        { text: texts[index]!, needsIdentity: counts.get(texts[index]!)! > 1 },
      ])
    ),
    columns: keys.map(key => ({
      key,
      cells: new Map(
        series.map((row, index) => [
          row.seriesRef,
          {
            preview: previews[index]!.get(key)?.text ?? '[absent]',
            kind: previews[index]!.get(key)?.kind,
            values: [],
            label: previews[index]!.get(key)?.text ?? '[absent]',
            signature: '',
          },
        ])
      ),
    })),
  }
}

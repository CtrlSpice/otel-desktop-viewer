export type StickyTreeRow = {
  parent: number
  end: number
  hasChildren: boolean
}
export type StickyAncestor = { index: number; offset: number }

export function buildStickyTree(
  rows: readonly { depth: number }[]
): StickyTreeRow[] {
  const result: StickyTreeRow[] = []
  const open: number[] = []
  for (let index = 0; index < rows.length; index++) {
    const depth = Math.max(0, rows[index].depth)
    while (open.length && rows[open[open.length - 1]].depth >= depth)
      result[open.pop()!].end = index
    const candidate = open[open.length - 1]
    const parent =
      candidate !== undefined && rows[candidate].depth === depth - 1
        ? candidate
        : -1
    if (parent < 0) while (open.length) result[open.pop()!].end = index
    result.push({ parent, end: rows.length, hasChildren: false })
    if (parent >= 0) result[parent].hasChildren = true
    open.push(index)
  }
  return result
}

export function stickyAncestors(
  tree: readonly StickyTreeRow[],
  scrollTop: number,
  rowHeight: number,
  maxHeight = Infinity
): StickyAncestor[] {
  if (
    !tree.length ||
    scrollTop <= 0 ||
    !Number.isFinite(scrollTop) ||
    rowHeight <= 0 ||
    !Number.isFinite(rowHeight) ||
    scrollTop >= tree.length * rowHeight ||
    Number.isNaN(maxHeight) ||
    maxHeight < 0
  )
    return []
  const pinned: StickyAncestor[] = []
  let previousFirst = -1
  let path: number[] = []
  for (let level = 0; level < tree.length; level++) {
    const slot = Math.min(level * rowHeight, maxHeight)
    const first = Math.min(
      tree.length - 1,
      Math.floor((scrollTop + slot) / rowHeight)
    )
    if (first !== previousFirst) {
      path = []
      for (
        let index = tree[first].hasChildren ? first : tree[first].parent;
        index >= 0;
        index = tree[index].parent
      )
        path.push(index)
      path.reverse()
      previousFirst = first
    }
    const index = path[level]
    if (
      index === undefined ||
      (level > 0 && tree[index].parent !== pinned[level - 1].index) ||
      index * rowHeight >= scrollTop + slot
    )
      break
    pinned.push({
      index,
      offset: Math.min(
        0,
        tree[index].end * rowHeight - scrollTop - slot - rowHeight
      ),
    })
  }
  return pinned
}

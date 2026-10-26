/** Toggle a complete group, retaining required columns and a minimum selection. */
export function updateColumnSelection(
  selected: readonly string[],
  options: readonly { id: string; disabled?: boolean }[],
  ids: readonly string[],
  checked: boolean,
  minimumSelected = 0
): string[] {
  const editable = new Set(
    options.flatMap(option =>
      !option.disabled && ids.includes(option.id) ? [option.id] : []
    )
  )
  if (checked) return [...new Set([...selected, ...editable])]
  const retained = new Set(selected.filter(id => !editable.has(id)))
  for (const id of selected) {
    if (retained.size >= minimumSelected) break
    retained.add(id)
  }
  return selected.filter(id => retained.has(id))
}

<script lang="ts">
  import TableColumnsPicker from '@/components/shared/TableColumnsPicker.svelte'
  import {
    COLUMN_SOURCES,
    columnID,
    columnLabel,
    type WaterfallColumn,
    type WaterfallColumnOption,
  } from './waterfall-columns'

  let {
    options,
    selected,
    onchange,
  }: {
    options: WaterfallColumnOption[]
    selected: WaterfallColumn[]
    onchange: (columns: WaterfallColumn[]) => void
  } = $props()
  const choices = $derived(
    COLUMN_SOURCES.flatMap(source =>
      [true, false].flatMap(field =>
        options
          .filter(
            option =>
              option.source === source && Boolean(option.field) === field
          )
          .map(option => ({
            id: option.id,
            label: option.key || '(empty key)',
            accessibleLabel: columnLabel(option),
            group: `${source} ${field ? 'fields' : 'attributes'}`,
            count: option.count,
            countTitle: option.count
              ? `${option.count} spans`
              : 'Not present in this trace',
          }))
      )
    )
  )

  function change(ids: string[]) {
    onchange(
      ids.flatMap(id => {
        const option = options.find(option => option.id === id)
        if (!option) return []
        const column: WaterfallColumn = {
          source: option.source,
          key: option.key,
        }
        if (option.field) column.field = true
        return [column]
      })
    )
  }
</script>

<TableColumnsPicker
  options={choices}
  selected={selected.map(columnID)}
  onchange={change}
  label="Waterfall columns"
/>

<script lang="ts">
  import { PAGE_SIZES, isPageSize, type PageSize } from '@/utils/page-size'

  let {
    label,
    value,
    onchange,
    disabled = false,
  }: {
    label: string
    value: PageSize
    onchange: (value: PageSize) => void
    disabled?: boolean
  } = $props()
</script>

<select
  class="select select-xs w-auto text-sm font-normal text-subtle"
  aria-label={label}
  {value}
  {disabled}
  onchange={event => {
    const next = Number(event.currentTarget.value)
    if (isPageSize(next)) onchange(next)
  }}
>
  {#each PAGE_SIZES as size}<option value={size}>{size} per page</option>{/each}
</select>

<script lang="ts">
  import { createSignalListPage } from '@/contexts/signal-list-page.svelte'
  import type { SignalListPage } from '@/contexts/signal-list-page.svelte'

  type Item = { id: string; name: string }

  interface Props {
    fetchList: () => Promise<Item[]>
    onContext?: (ctx: SignalListPage<Item>) => void
  }

  let { fetchList, onContext }: Props = $props()

  const page = createSignalListPage<Item>({
    signal: 'logs',
    getItemId: item => item.id,
    fetchList: () => fetchList(),
    compare: (a, b) => a.name.localeCompare(b.name),
    initialSort: { column: 'name', direction: 'asc' },
  })

  $effect(() => {
    onContext?.(page)
  })
</script>

<output data-testid="loading">{page.loading}</output>
<output data-testid="mounted">{page.mounted}</output>
<output data-testid="selected-id">{page.selectedId ?? ''}</output>
<output data-testid="selected-index">{page.selectedIndex}</output>
<output data-testid="item-count">{page.sortedItems.length}</output>
<output data-testid="item-ids">
  {page.sortedItems.map(item => item.id).join(',')}
</output>

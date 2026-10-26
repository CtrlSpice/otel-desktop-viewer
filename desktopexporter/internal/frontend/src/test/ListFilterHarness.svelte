<script lang="ts">
  import ListFilter from '@/components/shared/Search/ListFilter.svelte'
  let {
    label = 'Filter items',
    value = '',
    onkeydown,
  }: {
    label?: string
    value?: string
    onkeydown?: (event: KeyboardEvent) => void
  } = $props()
</script>

<ListFilter {label} bind:value {onkeydown}>
  {#snippet children(toggle, field)}
    <header>{@render toggle()}</header>
    {@render field()}
  {/snippet}
</ListFilter>

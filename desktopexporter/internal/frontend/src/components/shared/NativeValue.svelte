<script lang="ts">
  import { tick } from 'svelte'
  import type { AttributeValue } from '@/types/api-types'
  import NativeValueNode from './NativeValueNode.svelte'
  import JsonEditor from './JsonEditor.svelte'
  import { structuredValueText } from './detail-value'
  let { value, label }: { value: AttributeValue; label: string } = $props()
  let document = $state(false)
  let documentCreated = $state(false)
  let generation = $state(0)
  let openDepth = $state(1)
  let editor = $state<JsonEditor>()
  $effect(() => {
    void value
    document = false
    documentCreated = false
    openDepth = 1
  })
  const text = $derived(
    documentCreated ? (structuredValueText(value) ?? '') : ''
  )
  export function collapseAll() {
    if (document) editor?.collapseAll()
    else {
      openDepth = 0
      generation++
    }
  }
  export async function expandAll() {
    documentCreated = true
    document = true
    await tick()
    editor?.expandAll()
  }
  async function find() {
    documentCreated = true
    document = true
    await tick()
    editor?.find()
  }
</script>

<div class="native-value font-mono" aria-label={`Value of ${label}`}>
  {#if documentCreated}
    <div hidden={!document}>
      <button
        type="button"
        class="btn btn-ghost btn-sm font-sans font-normal"
        onclick={() => (document = false)}>Properties</button
      >
      <JsonEditor
        {text}
        label={`JSON value of ${label}`}
        defaultOpenDepth={1}
        bind:this={editor}
      />
    </div>
  {/if}
  <div hidden={document}>
    <button
      type="button"
      class="btn btn-ghost btn-sm font-sans font-normal"
      onclick={find}>Full document / Find</button
    >
    {#key generation}<NativeValueNode {value} {label} {openDepth} />{/key}
  </div>
</div>

<script lang="ts">
  import type { SpanData } from '@/types/api-types';
  import SpanField from './SpanField.svelte';

  type Props = {
    span: SpanData | undefined;
  };

  let { span }: Props = $props();

  let scopeAttributes = $derived(
    span
      ? Object.entries(span.scope.attributes).map(([key, value]) => ({
          key,
          name: key,
          value: value.toString(),
          type: 'type',
        }))
      : []
  );
</script>

{#if span}
  <div class="space-y-2">
    <div class="space-y-0">
      <SpanField
        fieldName="scope name"
        fieldValue={span.scope.name}
        fieldType="string"
      />
      <SpanField
        fieldName="scope version"
        fieldValue={span.scope.version}
        fieldType="string"
      />
      {#each scopeAttributes as attr}
        <SpanField
          fieldName={attr.name}
          fieldValue={attr.value}
          fieldType={attr.type}
        />
      {/each}
      {#if span.scope.droppedAttributesCount > 0}
        <SpanField
          fieldName="dropped attributes count"
          fieldValue={span.scope.droppedAttributesCount.toString()}
          fieldType="uint32"
        />
      {/if}
    </div>
  </div>
{:else}
  <p class="text-base-content/60">Nothing here yet.</p>
{/if}

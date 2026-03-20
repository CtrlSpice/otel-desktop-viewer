<script lang="ts">
  import type { SpanData } from '@/types/api-types';
  import SpanField from './SpanField.svelte';

  type Props = {
    span: SpanData | undefined;
  };

  let { span }: Props = $props();

  let resourceAttributes = $derived(span?.resource.attributes ?? []);
</script>

{#if span}
  <div class="space-y-2">
    <div class="space-y-0">
      {#each resourceAttributes as attr}
        <SpanField
          fieldName={attr.key}
          fieldValue={attr.value}
          fieldType={attr.type}
        />
      {/each}
      {#if span.resource.droppedAttributesCount > 0}
        <SpanField
          fieldName="dropped attributes count"
          fieldValue={span.resource.droppedAttributesCount.toString()}
          fieldType="uint32"
        />
      {/if}
    </div>
  </div>
{:else}
  <p class="text-base-content/60">Nothing here yet.</p>
{/if}

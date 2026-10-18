<script lang="ts">
  import type { Attributes } from '@/types/api-types'
  import DetailProperty from './DetailProperty.svelte'

  type Props = {
    attributes: Attributes
    owner: string
    /** Display context only; the received key is unchanged. */
    origin?: string
  }
  let { attributes, owner, origin }: Props = $props()
</script>

{#each attributes as attribute, index (`${owner}:${attribute.id ?? 'unidentified'}:${index}`)}
  <DetailProperty
    fieldName={attribute.key}
    fieldType={attribute.value.kind}
    typedValue={attribute.value}
    {origin}
    defectLabel={attribute.hasConflict
      ? `Conflicting typed values retained for ${attribute.key} in ${owner}.`
      : undefined}
  />
{/each}

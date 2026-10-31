<script lang="ts">
  import type { Attributes } from '@/types/api-types'
  import DetailProperty from './DetailProperty.svelte'

  type Props = {
    attributes: Attributes
    owner: string
    /** Display context only; the received key is unchanged. */
    origin?: string
    source?: string
  }
  let { attributes, owner, origin, source }: Props = $props()
</script>

{#each attributes as attribute, index (`${owner}:${attribute.id ?? 'unidentified'}:${index}`)}
  <DetailProperty
    fieldName={attribute.key}
    fieldType={attribute.value.kind}
    typedValue={attribute.value}
    fieldKind="attribute"
    fieldSource={source ?? origin ?? owner}
    {origin}
  />
{/each}

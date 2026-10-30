<script lang="ts">
  import DetailField from '@/components/shared/DetailProperty.svelte'
  import type { EvalSpanEvidence } from './eval-span-evidence'

  let { evidence }: { evidence: EvalSpanEvidence[] } = $props()
  const outputField = (key: string) =>
    /^(output|stdout|stderr|tool\.(output|result|error)|gen_ai\.tool\.call\.result)$/.test(
      key
    )
  const records = $derived(
    evidence
      .map(record => ({
        ...record,
        fields: [...record.fields].sort(
          (a, b) => Number(outputField(b.key)) - Number(outputField(a.key))
        ),
      }))
      .sort(
        (a, b) =>
          Number(b.fields.some(field => outputField(field.key))) -
          Number(a.fields.some(field => outputField(field.key)))
      )
  )
</script>

{#each records as record (record.id)}
  <table class="detail-fields w-full" aria-label={record.source}>
    <caption
      title={record.sourceDescription}
      class="px-[var(--pane-inline-inset)] py-2 text-left text-sm font-normal text-secondary"
      >{record.source}{#if record.toolError}<span class="text-subtle">
          ·
        </span><span class="text-base-content">Tool error</span>{/if}</caption
    >
    <tbody>
      {#each record.fields as attribute, index (`${attribute.key}/${index}`)}
        <DetailField
          fieldName={attribute.key}
          fieldType={attribute.value.kind}
          typedValue={attribute.value}
          fieldKind="attribute"
        />
      {/each}
    </tbody>
  </table>
{/each}

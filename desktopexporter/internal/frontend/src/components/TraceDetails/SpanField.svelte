<script lang="ts">
  type Props = {
    fieldType: string;
    fieldName: string;
    fieldValue: string;
    hidden?: boolean;
  };

  let { fieldType, fieldName, fieldValue, hidden = false }: Props = $props();
</script>

{#if !hidden}
  <div class="py-1 px-4">
    <dt class="flex items-center gap-2 mb-1">
      <span class="text-sm text-base-content/70">{fieldName}</span>
      <span class="badge badge-secondary badge-outline badge-sm text-xs"
        >{fieldType}</span
      >
    </dt>
    <dd>
      <p class="text-base py-1 break-words">{fieldValue}</p>
    </dd>
  </div>
{/if}

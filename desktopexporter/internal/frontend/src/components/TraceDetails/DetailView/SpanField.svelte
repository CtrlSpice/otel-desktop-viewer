<script lang="ts">
  type Props = {
    fieldType: string
    fieldName: string
    fieldValue: string
    origin?: 'resource' | 'scope'
    hidden?: boolean
    isRoot?: boolean
  }

  let {
    fieldType,
    fieldName,
    fieldValue,
    origin,
    hidden = false,
    isRoot = false,
  }: Props = $props()
</script>

{#if !hidden}
  <div class="span-field">
    <div class="span-field__key">
      <span class="span-field__name">{fieldName}</span>
      <span class="badge-type">{fieldType}</span>
      {#if origin}
        <span class="badge-origin">{origin}</span>
      {/if}
      {#if isRoot}
        <span class="badge-root">root</span>
      {/if}
    </div>
    <div class="span-field__value">
      {fieldValue}
    </div>
  </div>
{/if}

<style lang="postcss">
  .span-field {
    @apply border-t border-base-300/50;
  }

  .span-field:first-child {
    @apply border-t-0;
  }

  .span-field__key {
    @apply flex items-center gap-1 px-4 pt-1.5 pb-0.5 text-xs font-medium text-base-content/70;
  }

  .span-field__name {
    @apply mr-1;
  }

  .span-field__value {
    @apply px-4 pt-0.5 pb-1.5 text-sm text-base-content break-words;
  }
</style>

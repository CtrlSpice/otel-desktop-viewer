<script module lang="ts">
  export type RawJsonStatusData = {
    label: string
    description: string
    url?: string
  }
</script>

<script lang="ts">
  let { status }: { status: RawJsonStatusData } = $props()
  const hintID = $props.id()
</script>

<span
  class="cursor-help whitespace-nowrap text-sm font-normal text-rp-subtle"
  role="status"
  title={status.description}
  aria-describedby={hintID}>{status.label}</span
>
<span id={hintID} class="sr-only">{status.description}</span>

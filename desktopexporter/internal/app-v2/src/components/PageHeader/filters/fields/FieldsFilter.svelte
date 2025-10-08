<script lang="ts">
  // This will be implemented later with signal-specific field options
  export let fields: string[] = [];
  export let onFieldsChange: ((fields: string[]) => void) | null = null;

  // Dummy field options for now
  let availableFields = [
    'Name',
    'Service',
    'Duration',
    'Status',
    'Timestamp',
    'Trace ID',
    'Span ID',
  ];
</script>

<div class="form-control">
  <div class="flex justify-between items-center mb-2">
    <span class="text-sm font-medium text-base-content/80">Display Fields</span>
  </div>

  <div class="space-y-2">
    {#each availableFields as field}
      <label class="flex items-center gap-2">
        <input
          type="checkbox"
          class="checkbox checkbox-sm"
          checked={fields.includes(field)}
          onchange={e => {
            let target = e.target as HTMLInputElement;
            let newFields = target.checked
              ? [...fields, field]
              : fields.filter(f => f !== field);
            onFieldsChange?.(newFields);
          }}
        />
        <span class="text-sm">{field}</span>
      </label>
    {/each}
  </div>
</div>

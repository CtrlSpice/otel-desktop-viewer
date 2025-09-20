<script lang="ts">
  import type { AttributeFilter } from '../types/filter-types';
  import {
    ALL_ATTRIBUTE_SUGGESTIONS,
    searchSuggestions,
  } from '../constants/semantic-conventions';

  export let attributes: AttributeFilter[] = [];
  export let onAttributesChange:
    | ((attributes: AttributeFilter[]) => void)
    | null = null;

  // Attribute filter helpers
  function addAttributeFilter() {
    const newAttributes = [
      ...attributes,
      { name: '', value: '', operator: 'equals' as const },
    ];
    if (onAttributesChange) onAttributesChange(newAttributes);
  }

  function removeAttributeFilter(index: number) {
    const newAttributes = attributes.filter((_, i) => i !== index);
    if (onAttributesChange) onAttributesChange(newAttributes);
  }

  function clearAllFilters() {
    if (onAttributesChange) onAttributesChange([]);
  }

  // Search for attribute suggestions
  function getAttributeSuggestions(query: string) {
    if (!query) return ALL_ATTRIBUTE_SUGGESTIONS.slice(0, 10); // Show first 10 when empty
    return searchSuggestions(query).slice(0, 10); // Limit to 10 results
  }
</script>

<div class="form-control">
  <div class="flex justify-between items-center mb-2">
    <span class="text-sm font-medium text-base-content/80"
      >Attributes and Fields</span
    >
    <div class="flex gap-2">
      <button class="btn btn-outline btn-xs" onclick={addAttributeFilter}>
        + Add filter
      </button>
      <button
        class="text-xs text-base-content/60 hover:text-base-content underline"
        onclick={clearAllFilters}
      >
        Clear all
      </button>
    </div>
  </div>

  {#each attributes as attrFilter, index}
    <div class="flex gap-2 items-end mb-2">
      <div class="form-control flex-1">
        <label class="label" for="attr-name-{index}">
          <span class="label-text text-xs">Attribute Name</span>
        </label>
        <input
          id="attr-name-{index}"
          type="text"
          placeholder="e.g., http.status_code"
          class="input input-bordered input-sm"
          value={attrFilter.name}
          oninput={e => {
            const target = e.target as HTMLInputElement;
            const newAttributes = attributes.map((attr, i) =>
              i === index ? { ...attr, name: target.value } : attr
            );
            if (onAttributesChange) onAttributesChange(newAttributes);
          }}
          list="attribute-suggestions-{index}"
        />
        <datalist id="attribute-suggestions-{index}">
          {#each getAttributeSuggestions(attrFilter.name) as suggestion}
            <option value={suggestion.name} title={suggestion.description}>
              {suggestion.name} ({suggestion.category})
            </option>
          {/each}
        </datalist>
      </div>

      <div class="form-control w-32">
        <label class="label" for="attr-operator-{index}">
          <span class="label-text text-xs">Operator</span>
        </label>
        <select
          id="attr-operator-{index}"
          class="select select-bordered select-sm"
          value={attrFilter.operator}
          onchange={e => {
            const target = e.target as HTMLSelectElement;
            const newAttributes = attributes.map((attr, i) =>
              i === index
                ? {
                    ...attr,
                    operator: target.value as
                      | 'equals'
                      | 'contains'
                      | 'startsWith',
                  }
                : attr
            );
            if (onAttributesChange) onAttributesChange(newAttributes);
          }}
        >
          <option value="equals">equals</option>
          <option value="contains">contains</option>
          <option value="startsWith">starts with</option>
        </select>
      </div>

      <div class="form-control flex-1">
        <label class="label" for="attr-value-{index}">
          <span class="label-text text-xs">Value</span>
        </label>
        <input
          id="attr-value-{index}"
          type="text"
          placeholder="Filter value"
          class="input input-bordered input-sm"
          value={attrFilter.value}
          oninput={e => {
            const target = e.target as HTMLInputElement;
            const newAttributes = attributes.map((attr, i) =>
              i === index ? { ...attr, value: target.value } : attr
            );
            if (onAttributesChange) onAttributesChange(newAttributes);
          }}
        />
      </div>

      <button
        class="btn btn-error btn-sm"
        onclick={() => removeAttributeFilter(index)}
      >
        ×
      </button>
    </div>
  {/each}

  {#if !attributes || attributes.length === 0}
    <div class="text-center py-4 text-base-content/60">
      <p>No attribute filters added</p>
      <p class="text-sm">Click "Add Filter" to search by span attributes</p>
    </div>
  {/if}
</div>

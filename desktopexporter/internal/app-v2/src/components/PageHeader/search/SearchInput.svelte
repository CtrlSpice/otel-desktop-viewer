<script lang="ts">
  import { getFieldsBySignal, type FieldDefinition } from '@/constants/fields';
  import { type Operator } from '@/constants/operators';

  type Query = {
    fieldOperator?: {
      field: FieldDefinition;
      operator: Operator;
    };
    value: string | number | boolean | string[] | number[] | boolean[];
  };

  let {
    signal,
    view,
    placeholder,
  }: {
    signal: 'traces' | 'logs' | 'metrics';
    view: 'list' | 'detail';
    placeholder?: string;
  } = $props();

  // Single Query Building State
  let inputValue = $state(''); // The main input value
  let query = $state<Query | null>(null);

  // UI State
  let showSuggestions = $state(false);
  let fieldSuggestions = $state<FieldDefinition[]>([]);
  let debounceTimer: ReturnType<typeof setTimeout> | null = null;

  // Get available fields once at component creation
  const availableFields = getFieldsBySignal(signal, view);

  // Effect to show/hide fields popover
  $effect(() => {
    const popover = document.getElementById('fields-popover');
    if (popover) {
      if (showSuggestions && fieldSuggestions.length > 0) {
        popover.showPopover();
      } else {
        popover.hidePopover();
      }
    }
  });

  // Generate placeholder text based on signal
  function getPlaceholderText(): string {
    if (placeholder) return placeholder;
    if (signal) return `Search ${signal}...`;
    return 'Search...';
  }

  function handleInput(event: Event) {
    const target = event.target as HTMLInputElement;
    inputValue = target.value;

    // Clear existing debounce timer
    if (debounceTimer) {
      clearTimeout(debounceTimer);
    }

    // Debounce the suggestion logic
    debounceTimer = setTimeout(() => {
      // Early exit for very short queries
      if (inputValue.length < 2) {
        showSuggestions = false;
        fieldSuggestions = [];
        return;
      }

      // 1. We check if input matches a known pattern (trace ID, span ID)
      fieldSuggestions = detectPatternFields(inputValue);
      if (fieldSuggestions.length > 0) {
        // If no query yet, create one, otherwise update the existing one
        // Note to Mila: the spread operator preserves an existing fieldOperator
        // so things don't get wonky if the user types a field name first,
        // then pastes a patterned value
        query = !query
          ? { value: inputValue }
          : { ...query, value: inputValue };
        showSuggestions = fieldSuggestions.length > 0;
      } else if (!query?.fieldOperator?.field) {
        // No field selected yet: show field name suggestions
        fieldSuggestions = fuzzyMatchFields(inputValue);
        showSuggestions = fieldSuggestions.length > 0;
      } else {
        // Field is selected: treat input as value
        query.value = inputValue;
        showSuggestions = false;
      }
    }, 150);
  }

  function handleKeydown(event: KeyboardEvent) {
    if (event.key === 'Enter') {
      if (query) {
        onSearch(query);
      }
    } else if (event.key === 'Backspace' && inputValue === '') {
      // Reset on backspace when empty
      resetQuery();
    }
  }

  function acceptFieldSuggestion(field: FieldDefinition) {
    showSuggestions = false;
    query = !query
      ? { fieldOperator: { field, operator: field.operators[0] }, value: '' }
      : { ...query, fieldOperator: { field, operator: field.operators[0] } };
    
    inputValue = query.value as string;
    const popover = document.getElementById('fields-popover');
    if (popover) {
      popover.hidePopover();
    }
  }

  function selectOperator(operator: Operator) {
    if (!query?.fieldOperator) {
      return;
    }

    query = {
      ...query,
      fieldOperator: { field: query.fieldOperator.field, operator },
    };
    const popover = document.getElementById('operator-popover');
    if (popover) {
      popover.hidePopover();
    }
  }

  function resetQuery() {
    query = null;
    inputValue = '';
    showSuggestions = false;
    fieldSuggestions = [];
  }

  // Get field suggestions for detected patterns
  function detectPatternFields(input: string): FieldDefinition[] {
    const traceIdPattern = /^[a-f0-9]{32}$/i;
    const spanIdPattern = /^[a-f0-9]{16}$/i;

    if (traceIdPattern.test(input)) {
      return fuzzyMatchFields('traceId');
    }

    if (spanIdPattern.test(input)) {
      return fuzzyMatchFields('spanId');
    }

    return [];
  }

  // Fuzzy match field names
  function fuzzyMatchFields(query: string): FieldDefinition[] {
    const matches = availableFields
      .map(field => ({
        field,
        score: (() => {
          const lowerQuery = query.toLowerCase();
          const lowerFieldName = field.name.toLowerCase();
          const lowerDescription = field.description.toLowerCase();

          if (lowerFieldName === lowerQuery) return 4;
          if (lowerFieldName.startsWith(lowerQuery)) return 3;
          if (lowerFieldName.includes(lowerQuery)) return 2;
          if (lowerDescription.includes(lowerQuery)) return 1;
          return 0;
        })(),
      }))
      .filter(match => match.score > 0)
      .sort((a, b) => b.score - a.score)
      .slice(0, 10);

    return matches.map(match => match.field);
  }

  function onSearch(query: Query) {
    console.log(query);
  }
</script>

<div class="form-control flex-1">
  <div class="relative" style="anchor-name: --search-anchor">
    <!-- Token-based search container -->
    <div class="search-container">
      <svg
        class="search-icon"
        fill="none"
        stroke="currentColor"
        viewBox="0 0 24 24"
      >
        <path
          stroke-linecap="round"
          stroke-linejoin="round"
          stroke-width="2"
          d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"
        ></path>
      </svg>

      <!-- Field + Operator Token (shown when field is selected) -->
      {#if query?.fieldOperator}
        <div class="field-token" style="anchor-name: --operator-anchor">
          <button
            class="token-close"
            onclick={resetQuery}
            aria-label="Clear search">×</button
          >
          <span class="field-name">{query.fieldOperator.field.name}</span>
          <button
            id="operator-token"
            class="operator-token"
            aria-label="Change operator"
            popovertarget="operator-popover"
            style="anchor-name: --operator-anchor"
          >
            {query.fieldOperator.operator.symbol}
          </button>
        </div>
      {/if}

      <!-- Main Input -->
      <input
        id="search-input"
        type="text"
        placeholder={getPlaceholderText()}
        class="value-input"
        value={inputValue}
        oninput={handleInput}
        onkeydown={handleKeydown}
      />
    </div>

    <!-- Field Suggestions Popover -->
    <ul id="fields-popover" class="fields-popover" popover="hint">
      {#each fieldSuggestions as field, index}
        <li>
          <button
            class="list-button group"
            onclick={() => acceptFieldSuggestion(field)}
          >
            <div class="flex-1 min-w-0">
              <div class="text-base-content/90 font-medium">
                {field.name}
              </div>
              {#if field.description}
                <div class="description-slide">
                  {field.description}
                </div>
              {/if}
            </div>
          </button>
        </li>
        {#if index < fieldSuggestions.length - 1}
          <div class="border-t border-base-300"></div>
        {/if}
      {/each}
    </ul>

    <!-- Operator Selection Popover -->
    {#if query?.fieldOperator}
      <ul id="operator-popover" class="operator-popover" popover="hint">
        {#each query.fieldOperator.field.operators as operator}
          <li>
            <button onclick={() => selectOperator(operator)}
              >{operator.label}</button
            >
          </li>
        {/each}
      </ul>
    {/if}
  </div>
</div>

<style>
  /* Token-based Search Container */
  .search-container {
    /* Layout */
    @apply flex items-center gap-2;
    @apply input input-bordered input-sm;
    @apply w-full;

    /* Override input padding to accommodate tokens */
    @apply px-2 py-1;
  }

  .search-icon {
    @apply w-4 h-4 flex-shrink-0;
    @apply text-base-content/60;
  }

  /* Field Token - Pill with primary border, operator closes it off */
  .field-token {
    @apply flex items-center gap-1.5;
    @apply border border-secondary;
    @apply bg-base-100;
    @apply pl-1.5 pr-0;
    @apply h-6;
    @apply rounded-full;
    @apply text-xs font-medium;
    @apply flex-shrink-0;
  }

  .token-close {
    @apply flex items-center justify-center;
    @apply w-4 h-4;
    @apply text-xs;
    @apply text-base-content/50;
    @apply bg-transparent;
    @apply rounded-full;
    @apply cursor-pointer;
    @apply transition-all duration-150;
    @apply hover:text-base-content/80 hover:bg-base-200;
  }

  .field-name {
    @apply text-base-content;
  }

  /* Operator Token - Circle that closes off the pill */
  .operator-token {
    @apply flex items-center justify-center;
    @apply bg-secondary/40 text-secondary-content;
    @apply w-6 h-6;
    @apply rounded-full;
    @apply text-xs font-mono font-semibold;
    @apply transition-all duration-150;
    @apply hover:bg-secondary;
  }

  /* Value Input */
  .value-input {
    @apply flex-1;
    @apply bg-transparent;
    @apply border-none outline-none;
    @apply text-sm;
    @apply min-w-0;
    @apply placeholder:text-base-content/40;
  }

  .fields-popover {
    /* Layout & Positioning */
    @apply dropdown-content;
    @apply min-w-20 mx-0 my-2;
    position-anchor: --search-anchor;
    top: anchor(--search-anchor bottom);
    left: anchor(--search-anchor left);
    width: anchor(--search-anchor width);

    /* Visual Styling */
    @apply bg-base-100 rounded-md shadow-lg;
    @apply border border-base-300 text-base-content;
  }

  .list-button {
    @apply w-full px-3 py-2;
    @apply text-left;
    @apply transition-colors duration-150;
    @apply hover:bg-base-200;
  }

  .description-slide {
    /* Layout & Spacing */
    @apply text-xs mt-1;
    @apply opacity-0 max-h-0 overflow-hidden;

    /* Visual Styling */
    @apply text-base-content/60;

    /* Animation */
    @apply transform transition-all duration-200 ease-out;
    @apply group-hover:opacity-100 group-hover:max-h-8;
  }

  /* Operator Popover */
  .operator-popover {
    @apply dropdown-content;
    @apply mx-0 my-2;
    position-anchor: --operator-anchor;
    top: anchor(--operator-anchor bottom);
    left: anchor(--operator-anchor left);
    @apply bg-base-100 rounded-md shadow-lg;
    @apply border border-base-300 text-base-content;
    @apply p-3 min-w-32;
  }

  /* Disabled input state */
  input:disabled {
    @apply cursor-not-allowed opacity-0;
  }
</style>

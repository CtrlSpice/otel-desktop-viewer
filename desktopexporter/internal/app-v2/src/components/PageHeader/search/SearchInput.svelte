<script lang="ts">
  import { getFieldsBySignal, type FieldDefinition } from '@/constants/fields';
  import { type Operator } from '@/constants/operators';
  import { 
    type Query, 
    type LogicalOperator, 
    type QueryNode,
    addConditionToTree,
    removeConditionFromTree,
  } from './query-tree';

  let {
    signal,
    view,
    placeholder,
  }: {
    signal: 'traces' | 'logs' | 'metrics';
    view: 'list' | 'detail';
    placeholder?: string;
  } = $props();

  // Multi-Query Building State
  let inputValue = $state(''); // The main input value
  let query = $state<Query | null>(null);
  let queryTree = $state<QueryNode | null>(null);

  // UI State
  let showSuggestions = $state(false);
  let fieldSuggestions = $state<FieldDefinition[]>([]);
  let debounceTimer: ReturnType<typeof setTimeout> | null = null;

  // Get available fields once at component creation
  const availableFields = getFieldsBySignal(signal, view);

  // Effect to show/hide suggestions popover
  $effect(() => {
    const popover = document.getElementById('suggestions-popover');
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
      } else if (!query?.predicate?.field) {
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
        onSearch();
      }
    } else if (event.key === 'Backspace' && inputValue === '') {
      // Reset on backspace when empty
      resetQuery();
    }
  }

  function acceptFieldSuggestion(field: FieldDefinition) {
    showSuggestions = false;
    query = !query
      ? { predicate: { field, operator: field.operators[0] }, value: '' }
      : { ...query, predicate: { field, operator: field.operators[0] } };

    inputValue = query.value as string;
    const popover = document.getElementById('suggestions-popover');
    if (popover) {
      popover.hidePopover();
    }
  }

  function selectOperator(operator: Operator) {
    if (!query?.predicate) {
      return;
    }

    query = {
      ...query,
      predicate: { field: query.predicate.field, operator },
    };
    const popover = document.getElementById('operator-popover');
    if (popover) {
      popover.hidePopover();
    }
  }

  function resetFieldOperator() {
    query = query ? { ...query, predicate: undefined } : null;
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

  // Check if we should show AND/OR buttons
  function shouldShowLogicalOperators(): boolean {
    return inputValue.trim() !== '';
  }

  // Add a new condition with logical operator
  function addCondition(logicalOperator: LogicalOperator) {
    if (!query) return;

    const newCondition: Query = {
      predicate: query.predicate,
      value: query.value
    };

    queryTree = addConditionToTree(queryTree, newCondition, logicalOperator);

    // Reset current query for next condition
    query = null;
    inputValue = '';

    // Close the popover
    const popover = document.getElementById('condition-popover');
    if (popover) {
      popover.hidePopover();
    }
  }

  // Remove a condition by ID
  function removeCondition(id: string) {
    queryTree = removeConditionFromTree(queryTree, id);
  }

  function onSearch() {
    console.log('QueryTree:', $state.snapshot(queryTree));
  }
</script>

<div class="form-control flex-1">
  <div class="relative" style="anchor-name: --search-anchor">
    <!-- Token-based search container -->
    <div class="search-container">
      <svg
        class="search-icon"
        xmlns="http://www.w3.org/2000/svg"
        width="16"
        height="16"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-linecap="round"
        stroke-linejoin="round"
        stroke-width="1.5"
      >
        <path d="m17 17l4 4m-2-10a8 8 0 1 0-16 0a8 8 0 0 0 16 0"/>
      </svg>

      <!-- Field + Operator Token (shown when field is selected) -->
      {#if query?.predicate}
        <div class="field-token">
          <button
            class="token-close"
            onclick={resetFieldOperator}
            aria-label="Clear search">×</button
          >
          <span class="field-name">{query.predicate.field.name}</span>
          <button
            id="operator-token"
            class="operator-token"
            aria-label="Change operator"
            popovertarget="operator-popover"
            style="anchor-name: --operator-anchor"
          >
            {query.predicate.operator.symbol}
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

      <!-- Add Condition Button (shown when there's content) -->
      {#if shouldShowLogicalOperators()}
        <button
          class="add-condition-btn"
          popovertarget="condition-popover"
          aria-label="Add condition"
        >
          <svg
            class="w-3 h-3"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M12 6v6m0 0v6m0-6h6m-6 0H6"
            />
          </svg>
        </button>
      {/if}
    </div>

    <!-- Field Suggestions Popover -->
    <ul id="suggestions-popover" class="suggestions-popover" popover="hint">
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
    {#if query?.predicate}
      <ul id="operator-popover" class="operator-popover" popover="hint">
        {#each query.predicate.field.operators as operator}
          <li>
            <button
              class="list-button group"
              onclick={() => selectOperator(operator)}
            >
              {operator.label}
            </button>
          </li>
        {/each}
      </ul>
    {/if}

    <!-- Condition Popover -->
    <div id="condition-popover" class="popover condition-popover" popover="hint">
      <button
        class="list-button group"
        onclick={() => addCondition('AND')}
      >
        AND
      </button>
      <button
        class="list-button group"
        onclick={() => addCondition('OR')}
      >
        OR
      </button>
    </div>
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
    @apply text-base-content;
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

  .suggestions-popover {
    /* Layout & Positioning */
    @apply dropdown-content;
    @apply min-w-20;
    position-anchor: --search-anchor;
    top: anchor(--search-anchor bottom);
    left: anchor(--search-anchor left);
    width: anchor(--search-anchor width);

    /* Visual Styling */
    @apply bg-base-100 rounded-md shadow-lg;
    @apply border border-base-300 text-base-content;
    @apply p-0 mx-0 my-2;
  }

  .operator-popover {
    /* Layout & Positioning */
    @apply min-w-10;
    position-anchor: --operator-anchor;
    top: anchor(--operator-anchor bottom);
    justify-self: anchor-center;

    /* Visual Styling */
    @apply bg-base-100 rounded-md shadow-lg;
    @apply border border-base-300 text-base-content;
    @apply p-0 mx-0 my-2;
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

  /* Add Condition Button */
  .add-condition-btn {
    @apply flex items-center justify-center;
    @apply w-6 h-6;
    @apply bg-base-100;
    @apply border border-dashed border-secondary;
    @apply rounded-full;
    @apply text-base-content/50;
    @apply transition-all duration-150;
    @apply hover:bg-base-200 hover:border-solid hover:text-base-content;
    @apply active:scale-95;
  }

  /* Condition Popover */
  .condition-popover {
    @apply dropdown-content;
    @apply min-w-20;
    position-anchor: --search-anchor;
    top: anchor(--search-anchor bottom);
    left: anchor(--search-anchor right);
    transform: translateX(-100%);
    @apply bg-base-100 rounded-md shadow-lg;
    @apply border border-base-300 text-base-content;
    @apply p-0 mx-0 my-2;
  }

  /* Disabled input state */
  input:disabled {
    @apply cursor-not-allowed opacity-0;
  }
</style>

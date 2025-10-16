<script lang="ts">
  import { getFieldsBySignal, type FieldDefinition } from '@/constants/fields';
  import {
    type Query,
    type QueryNode,
    type LogicalOperator,
    addConditionToTree,
    removeConditionFromTree,
    getAllConditions,
    getLogicalOperators,
    LOGICAL_OPERATORS,
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

  // Core search state
  let inputValue = $state(''); // The main input value
  let query = $state<Query | null>(null);
  let queryTree = $state<QueryNode | null>(null);
  let pendingLogicalOperator = $state<LogicalOperator | null>(null);

  // Field suggestions state
  let fieldSuggestions = $state<FieldDefinition[]>([]);
  let showSuggestions = $state(false);
  let debounceTimer: ReturnType<typeof setTimeout> | null = null;

  // Get available fields once at component creation
  const availableFields = getFieldsBySignal(signal, view);

  //#region INPUT HANDLING
  function handleInput(event: Event) {
    const target = event.target as HTMLInputElement;
    inputValue = target.value;
  }

  function handleKeydown(event: KeyboardEvent) {
    if (event.key === 'Enter') {
      onSearch();
    } else if (event.key === 'Backspace' && inputValue === '') {
      // Delete in order: input → predicate → pending operator → conditions (right to left)
      
      // 1. If we have a predicate (field selected), remove it first
      if (query?.predicate) {
        resetPredicate();
        return;
      }
      
      // 2. If we have a pending logical operator, clear it
      if (pendingLogicalOperator) {
        pendingLogicalOperator = null;
        return;
      }
      
      // 3. If we have conditions in the tree, remove the last one
      if (queryTree) {
        const allConditions = getAllConditions(queryTree);
        if (allConditions.length > 0) {
          const lastCondition = allConditions[allConditions.length - 1];
          queryTree = removeConditionFromTree(queryTree, lastCondition.id);
        } else {
          // No conditions left, reset the query
          resetQuery();
        }
      } else {
        // No tree, just reset the current query
        resetQuery();
      }
    }
  }

  // React to inputValue changes with debounce
  $effect(() => {
    // Clear existing debounce timer
    if (debounceTimer) {
      clearTimeout(debounceTimer);
    }

    // Early exit for very short queries
    if (inputValue.length < 2) {
      fieldSuggestions = [];
      showSuggestions = false;
      return;
    }
    
    // Debounce the suggestion logic
    debounceTimer = setTimeout(() => {
      // Check if input matches a known pattern (trace ID, span ID)
      const patternFields = detectPatternFields(inputValue);
      if (patternFields.length > 0) {
        fieldSuggestions = patternFields;
        showSuggestions = true;
      } else if (!query?.predicate?.field) {
        // No field selected yet: show field name suggestions
        const fuzzyFields = fuzzyMatchFields(inputValue);
        fieldSuggestions = fuzzyFields;
        showSuggestions = fuzzyFields.length > 0;
      } else {
        // Field is selected: treat input as value
        showSuggestions = false;
      }
    }, 150);
  });

  // Effect to show/hide field suggestions popover
  $effect(() => {
    const popover = document.getElementById('field-suggestions-popover');
    if (popover) {
      if (showSuggestions && fieldSuggestions.length > 0) {
        popover.showPopover();
      } else {
        popover.hidePopover();
      }
    }
  });
  //#endregion

  //#region PREDICATE MANAGEMENT

  // Select field for predicate
  function selectField(field: FieldDefinition) {
    // Update the current query with the selected field
    query = !query
      ? { predicate: { field, operator: field.operators[0] }, value: '' }
      : { ...query, predicate: { field, operator: field.operators[0] } };

    // Update input value to show the field selection
    inputValue = query.value as string;

    // Close the popover after selection
    const popover = document.getElementById('field-suggestions-popover');
    if (popover) {
      popover.hidePopover();
    }
  }

  // Select operator for predicate
  function selectOperator(operator: any) {
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

  // Reset predicate
  function resetPredicate() {
    query = query ? { ...query, predicate: undefined } : null;
  }
  //#endregion

  //#region LOGICAL OPERATORS

  // Handle logical operator selection
  function selectLogicalOperator(operator: LogicalOperator) {
    if (!query) {
      return;
    }
    try {
      // Sync query.value with inputValue before adding to tree
      syncQueryValue();
      
      queryTree = addConditionToTree(queryTree, query, pendingLogicalOperator || undefined);
    } catch (error) {
      console.error('Failed to add condition to tree:', error);
      return;
    }
    
    resetQuery();
    pendingLogicalOperator = operator;

    // Close the popover after selection
    const popover = document.getElementById('logical-operator-popover');
    if (popover) {
      popover.hidePopover();
    }
  }
  //#endregion

  //#region QUERY MANAGEMENT

  // Sync query.value with current inputValue
  function syncQueryValue() {
    if (!inputValue.trim()) {
      return;
    }
    query = query ? { ...query, value: inputValue.trim() } : { value: inputValue.trim() };

  }

  function onSearch() {
    syncQueryValue();
    if (query && query.value.trim()) {
      try {
        // Add condition to tree with pending logical operator if it exists
        queryTree = addConditionToTree(
          queryTree,
          query,
          pendingLogicalOperator || undefined
        );
      } catch (error) {
        console.error('Failed to add condition to tree:', error);
        return;
      }
    }
    resetQuery();

    console.log('QueryTree:', $state.snapshot(queryTree));
    //reset the tree
    queryTree = null;
  }

  function resetQuery() {
    query = null;
    inputValue = '';
    pendingLogicalOperator = null;
  }

  // Remove a condition by ID
  function removeCondition(id: string) {
    queryTree = removeConditionFromTree(queryTree, id);
  }
  //#endregion

  //#region UTILITY FUNCTIONS

  // Generate placeholder text based on signal
  function getPlaceholderText(): string {
    if (placeholder) return placeholder;
    if (signal) return `Search ${signal}...`;
    return 'Search...';
  }

  // Detect pattern fields (trace ID, span ID)
  function detectPatternFields(input: string): FieldDefinition[] {
    const traceIdPattern = /^[a-f0-9]{32}$/i;
    const spanIdPattern = /^[a-f0-9]{16}$/i;

    if (traceIdPattern.test(input)) {
      syncQueryValue();
      return fuzzyMatchFields('traceId');
    }

    if (spanIdPattern.test(input)) {
      syncQueryValue();
      return fuzzyMatchFields('spanId');
    }

    return [];
  }

  // Fuzzy match field names
  function fuzzyMatchFields(input: string): FieldDefinition[] {
    const matches = availableFields
      .map(field => ({
        field,
        score: (() => {
          const lowerInput = input.toLowerCase();
          const lowerFieldName = field.name.toLowerCase();
          const lowerDescription = field.description.toLowerCase();

          if (lowerFieldName === lowerInput) return 4;
          if (lowerFieldName.startsWith(lowerInput)) return 3;
          if (lowerFieldName.includes(lowerInput)) return 2;
          if (lowerDescription.includes(lowerInput)) return 1;
          return 0;
        })(),
      }))
      .filter(match => match.score > 0)
      .sort((a, b) => b.score - a.score)
      .slice(0, 10);

    return matches.map(match => match.field);
  }
  //#endregion
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
        <path d="m17 17l4 4m-2-10a8 8 0 1 0-16 0a8 8 0 0 0 16 0" />
      </svg>

      <!-- Existing Conditions (completed pills) -->
      {#if queryTree}
        {#each getAllConditions(queryTree) as condition, index}
          <div class="token-base completed-condition-token">
            <button
              class="token-close"
              onclick={() => removeCondition(condition.id)}
              aria-label="Remove condition"
            >
              <svg
                xmlns="http://www.w3.org/2000/svg"
                width="32"
                height="32"
                viewBox="0 0 24 24"
              >
                <path
                  fill="none"
                  stroke="currentColor"
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="1.5"
                  d="M18 6L6 18m12 0L6 6"
                />
              </svg>
            </button>
            <span class="field-name text-base-content"
              >{condition.query.predicate?.field.name}</span
            >
            <span class="operator-symbol"
              >{condition.query?.predicate?.operator.symbol}</span
            >
            <span class="text-base-content">{condition.query?.value}</span>
          </div>

          <!-- Logical Operator between conditions -->
          {#if index < getAllConditions(queryTree).length - 1}
            <span class="logical-operator-token">
              {getLogicalOperators(queryTree)[index]}
            </span>
          {/if}
        {/each}
      {/if}

      <!-- Pending Logical Operator (dashed) -->
      {#if pendingLogicalOperator}
        <span class="logical-operator-token pending">
          {pendingLogicalOperator}
        </span>
      {/if}

      <!-- Field + Operator Token (shown when field is selected) -->
      {#if query?.predicate}
        <div class="token-base field-token">
          <button
            class="token-close"
            onclick={resetPredicate}
            aria-label="Clear search"
          >
            <svg
              xmlns="http://www.w3.org/2000/svg"
              width="32"
              height="32"
              viewBox="0 0 24 24"
            >
              <path
                fill="none"
                stroke="currentColor"
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="1.5"
                d="M18 6L6 18m12 0L6 6"
              />
            </svg>
          </button>
          <span class="field-name text-base-content">{query.predicate.field.name}</span>
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

      <!-- Logical Operator Selector -->
      {#if inputValue.trim() !== ''}
        <!-- Logical Operator Button -->
        <button
          class="add-condition-btn"
          popovertarget="logical-operator-popover"
          aria-label="Add condition"
        >
          <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
            <path
              fill="none"
              stroke="currentColor"
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="1.5"
              d="M12 4v16m8-8H4"
            />
          </svg>
        </button>
      {/if}
    </div>

    <!-- Field Suggestions Popover -->
    <ul
      id="field-suggestions-popover"
      class="popover-base suggestions-popover"
      popover="auto"
    >
      {#each fieldSuggestions as field, index}
        <li>
          <button class="list-button group" onclick={() => selectField(field)}>
             <span>{field.name}</span>
          </button>
        </li>
      {/each}
    </ul>

    <!-- Operator Selection Popover -->
    {#if query?.predicate}
      <ul id="operator-popover" class="popover-base operator-popover" popover="auto">
        {#each query.predicate.field.operators as operator}
          <li>
            <button
              class="list-button group"
              onclick={() => selectOperator(operator)}
            >
              <span class="font-mono w-8">{operator.symbol}</span>
              <span>{operator.label}</span>
            </button>
          </li>
        {/each}
      </ul>
    {/if}

    <!-- Logical Operator Popover -->
    <div
      id="logical-operator-popover"
      class="popover-base popover logical-operator-popover"
      popover="hint"
    >
      {#each LOGICAL_OPERATORS as { operator, label }}
        <button
          class="list-button group"
          onclick={() => selectLogicalOperator(operator)}
        >
          {label}
        </button>
      {/each}
    </div>
  </div>
</div>

<style>
  /* Main Search Container */
  .search-container {
    @apply flex items-center gap-2;
    @apply input input-bordered input-sm;
    @apply w-full px-2 py-1;
  }

  .search-icon {
    @apply w-4 h-4 flex-shrink-0 text-base-content;
  }

  /* Token Elements */
  .token-base {
    @apply flex items-center gap-1.5;
    @apply border border-secondary;
    @apply bg-base-100;
    @apply h-6;
    @apply rounded-full;
    @apply text-xs font-medium;
    @apply flex-shrink-0;
  }

  /* Completed Condition Token */
  .token-base.completed-condition-token {
    @apply px-1;
  }

  /* Field Token */
  .token-base.field-token {
    @apply pl-1.5 pr-0;
  }

  /* Logical Operator Token */
  .logical-operator-token {
    @apply flex items-center justify-center;
    @apply bg-base-200 text-base-content/70;
    @apply px-2 py-1;
    @apply rounded-full;
    @apply text-xs font-medium;
    @apply flex-shrink-0;
  }

  .logical-operator-token.pending {
    @apply bg-transparent;
    @apply border border-dashed border-base-content/50;
    @apply text-base-content/60;
  }

  /* Token Content & Buttons */
  .operator-symbol {
    @apply text-secondary-content font-mono font-semibold;
  }

  .token-close {
    @apply flex items-center justify-center;
    @apply w-4 h-4 text-xs;
    @apply text-base-content/50 bg-transparent;
    @apply rounded-full cursor-pointer;
    @apply transition-all duration-150;
    @apply hover:text-base-content/80 hover:bg-base-200;
  }

  .token-close svg {
    @apply w-3 h-3;
  }

  .operator-token {
    @apply flex items-center justify-center;
    @apply bg-secondary/40 text-secondary-content;
    @apply w-6 h-6 rounded-full;
    @apply text-xs font-mono font-semibold;
    @apply transition-all duration-150;
    @apply hover:bg-secondary;
  }

  /* Action Buttons */
  .add-condition-btn {
    @apply flex items-center justify-center;
    @apply w-6 h-6 bg-base-100;
    @apply border border-dashed border-secondary;
    @apply rounded-full;
    @apply text-base-content/50;
    @apply transition-all duration-150;
    @apply hover:bg-base-200 hover:border-solid hover:text-base-content;
    @apply active:scale-95;
  }

  .add-condition-btn svg {
    @apply w-3 h-3;
  }

  /* Popovers */
  .popover-base {
    @apply bg-base-100 rounded-md shadow-lg;
    @apply border border-base-300 text-base-content;
    @apply p-0 mx-0 my-2;
    @apply min-w-40;
  }

  .popover-base.suggestions-popover {
    position-anchor: --search-anchor;
    top: anchor(--search-anchor bottom);
    left: anchor(--search-anchor left);
  }

  .popover-base.operator-popover {
    position-anchor: --operator-anchor;
    top: anchor(--operator-anchor bottom);
    justify-self: anchor-center;
  }

  .popover-base.logical-operator-popover {    
    position-anchor: --search-anchor;
    top: anchor(--search-anchor bottom);
    left: anchor(--search-anchor right);
    transform: translateX(-100%);
  }


  /* Input Elements */
  .value-input {
    @apply flex-1 bg-transparent;
    @apply border-none outline-none;
    @apply text-sm min-w-0;
    @apply placeholder:text-base-content/40;
  }
</style>

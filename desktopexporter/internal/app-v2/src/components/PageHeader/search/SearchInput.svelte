<script lang="ts">
  import {
    getFieldsBySignal,
    getAttributesBySignal,
    getAttributePrefixesBySignal,
    type FieldDefinition,
  } from '@/constants/fields';
  import { type QueryNode } from './queryTree';
  import { parseQuery } from './queryParser';

  // Component Props
  let {
    signal,
    placeholder,
  }: {
    signal: 'traces' | 'logs' | 'metrics';
    placeholder?: string;
  } = $props();

  // Core State
  let inputValue = $state(''); // Text input value
  let queryTree = $state<QueryNode | null>(null); // Parsed query tree
  let parseError = $state<string | null>(null); // Parser error message

  // Autocomplete State
  let fieldSuggestions = $state<FieldDefinition[]>([]);
  let showSuggestions = $state(false);
  let selectedSuggestionIndex = $state(0);
  let suggestionsDebounceTimer: ReturnType<typeof setTimeout> | null = null;

  // Get Available Fields For This Signal Type
  // Get Available Fields For This Signal Type
  const availableFields = [
    ...getFieldsBySignal(signal),
    ...getAttributesBySignal(signal),
  ];

  // Get Attribute Groups For Prefix-Aware Matching
  const attributeGroups = getAttributePrefixesBySignal(signal);


  // Handle Text Input Changes
  function handleInput(event: Event) {
    const target = event.target as HTMLInputElement;
    inputValue = target.value;

    // Clear any previous parse errors when user edits
    parseError = null;

    // Update autocomplete suggestions (debounced)
    updateSuggestionsDebounced();
  }

  // Handle Keyboard Events
  function handleKeydown(event: KeyboardEvent) {
    // Navigate autocomplete with arrow keys
    if (showSuggestions && fieldSuggestions.length > 0) {
      if (event.key === 'ArrowDown') {
        event.preventDefault();
        selectedSuggestionIndex = Math.min(
          selectedSuggestionIndex + 1,
          fieldSuggestions.length - 1
        );
        return;
      }

      if (event.key === 'ArrowUp') {
        event.preventDefault();
        selectedSuggestionIndex = Math.max(selectedSuggestionIndex - 1, 0);
        return;
      }

      if (event.key === 'Tab' || event.key === 'Enter') {
        // Select highlighted suggestion
        if (fieldSuggestions[selectedSuggestionIndex]) {
          event.preventDefault();
          selectField(fieldSuggestions[selectedSuggestionIndex]);
          return;
        }
      }

      if (event.key === 'Escape') {
        // Close suggestions
        showSuggestions = false;
        return;
      }
    }

    // Submit on Enter (if no suggestions open)
    if (event.key === 'Enter') {
      onSearch();
    }
  }



  // Update Field Suggestions With Debounce
  function updateSuggestionsDebounced() {
    if (suggestionsDebounceTimer) {
      clearTimeout(suggestionsDebounceTimer);
    }

    suggestionsDebounceTimer = setTimeout(() => {
      updateSuggestions();
    }, 150);
  }

  // Update Field Suggestions Based On Current Input
  function updateSuggestions() {
    // Minimum 2 characters to show suggestions
    if (inputValue.length < 2) {
      fieldSuggestions = [];
      showSuggestions = false;
      return;
    }

    // Check for pattern matches (trace ID, span ID) first
    let patterns = detectPatternFields(inputValue);
    if (patterns.length > 0) {
      fieldSuggestions = patterns;
      showSuggestions = true;
      selectedSuggestionIndex = 0;
      return;
    }

    // Determine if user is typing a field name
    let context = analyzeTypingContext(inputValue);

    if (context.expectingField && context.partial) {
      // Fuzzy match field names
      fieldSuggestions = fuzzyMatchFields(context.partial);
      showSuggestions = fieldSuggestions.length > 0;
      selectedSuggestionIndex = 0;
    } else {
      // Not expecting a field name
      fieldSuggestions = [];
      showSuggestions = false;
    }
  }

  // Analyze Typing Context To Determine If User Is Typing A Field Name
  function analyzeTypingContext(input: string): {
    expectingField: boolean;
    partial: string;
  } {
    const trimmed = input.trimEnd();

    // At start of input or after logical operator
    if (trimmed === '') {
      const partial = input.substring(trimmed.length).trim();
      return { expectingField: true, partial };
    }

    // After logical operator - only expect field if there's actual content after
    if (/\b(AND|OR)\s+$/i.test(trimmed)) {
      const partial = input.substring(trimmed.length).trim();
      return { expectingField: true, partial };
    }

    // After opening parenthesis
    if (/\(\s*$/.test(trimmed)) {
      const partial = input.substring(trimmed.length).trim();
      return { expectingField: true, partial };
    }

    // If last character is an operator, not expecting field
    if (/[:=!<>~\[\]]$/.test(trimmed)) {
      return { expectingField: false, partial: '' };
    }

    // Try to find the last token
    const lastWordMatch = input.match(/([\w.]+)$/);
    if (lastWordMatch) {
      const lastWord = lastWordMatch[1];
      
      // Don't treat logical operators as field names
      if (lastWord.toUpperCase() === 'AND' || lastWord.toUpperCase() === 'OR') {
        return { expectingField: false, partial: '' };
      }
      
      const beforeWord = input.substring(0, input.length - lastWord.length);

      // Check if there's an operator immediately before this word
      if (/[:=!<>~]\s*$/.test(beforeWord)) {
        return { expectingField: false, partial: '' };
      }

      // If there's no operator after the word yet, it might be a field
      if (!/:/.test(input.substring(input.length - lastWord.length))) {
        return { expectingField: true, partial: lastWord };
      }
    }

    return { expectingField: false, partial: '' };
  }

  // Detect Pattern Fields (Trace ID, Span ID)
  function detectPatternFields(input: string): FieldDefinition[] {
    const traceIdPattern = /\b([a-f0-9]{32})\b/i;
    const spanIdPattern = /\b([a-f0-9]{16})\b/i;

    // Check for trace ID
    const traceIdMatch = input.match(traceIdPattern);
    if (traceIdMatch) {
      return fuzzyMatchFields('traceId');
    }

    // Check for span ID
    const spanIdMatch = input.match(spanIdPattern);
    if (spanIdMatch) {
      return fuzzyMatchFields('spanId');
    }

    return [];
  }

  // Fuzzy Match Field Names with Prefix-Aware Matching
  function fuzzyMatchFields(input: string): FieldDefinition[] {
    const lowerInput = input.toLowerCase();

    // If input starts with a known prefix, prioritize that group
    const knownPrefixes = Object.keys(attributeGroups);
    const matchingPrefix = knownPrefixes.find(prefix =>
      lowerInput.startsWith(prefix + '.')
    );

    if (matchingPrefix) {
      // User is typing a specific prefix, focus on that group
      const groupFields = attributeGroups[matchingPrefix];
      return fuzzyMatchWithinFields(groupFields, input);
    }

    // Otherwise, search across all fields and attributes
    return fuzzyMatchWithinFields(availableFields, input);
  }

  // Helper function to do the actual fuzzy matching
  function fuzzyMatchWithinFields(
    fields: FieldDefinition[],
    input: string
  ): FieldDefinition[] {
    const matches = fields
      .map(field => ({
        field,
        score: calculateMatchScore(field, input),
      }))
      .filter(match => match.score > 0)
      .sort((a, b) => b.score - a.score)
      .slice(0, 10);

    return matches.map(match => match.field);
  }

  // Enhanced scoring for better attribute matching
  function calculateMatchScore(field: FieldDefinition, input: string): number {
    const lowerInput = input.toLowerCase();
    const lowerFieldName = field.name.toLowerCase();
    const lowerDescription = field.description.toLowerCase();

    // Exact match gets highest score
    if (lowerFieldName === lowerInput) return 10;

    // Prefix match gets high score
    if (lowerFieldName.startsWith(lowerInput)) return 8;

    // Contains match gets medium score
    if (lowerFieldName.includes(lowerInput)) return 5;

    // Description match gets low score
    if (lowerDescription.includes(lowerInput)) return 2;

    return 0;
  }

  // Select A Field From Suggestions
  function selectField(field: FieldDefinition) {
    // Find where to insert the field name
    const context = analyzeTypingContext(inputValue);

    if (context.partial) {
      // Replace the partial field name
      const beforePartial = inputValue.substring(
        0,
        inputValue.length - context.partial.length
      );
      inputValue = beforePartial + field.name + '=';
    } else {
      // Append field name
      inputValue = inputValue + field.name + '=';
    }

    // Close suggestions
    showSuggestions = false;

    // Focus input
    const input = document.getElementById('search-input') as HTMLInputElement;
    if (input) {
      input.focus();
    }
  }



  // Submit Search Query
  function onSearch() {
    // Parse final query
    try {
      queryTree = parseQuery(inputValue, availableFields);
      parseError = null;

      if (queryTree) {
        console.log('Submitting query:', $state.snapshot(queryTree));
        // TODO: Send to backend via JSON-RPC
      }
    } catch (error) {
      parseError = error instanceof Error ? error.message : 'Parse error';
    }
  }



  // Generate Placeholder Text
  function getPlaceholderText(): string {
    if (placeholder) return placeholder;

    switch (signal) {
      case 'traces':
        return 'status:200 AND name~error';
      case 'logs':
        return 'level:error AND body~timeout';
      case 'metrics':
        return 'name:cpu.usage AND type:Gauge';
      default:
        return 'Search...';
    }
  }


  // Effect: Show/Hide Suggestions Popover
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
</script>

<div class="form-control flex-1">
  <div class="relative" style="anchor-name: --search-anchor">
    <!-- Search Input Container -->
    <div class="search-container">
      <!-- Search Icon -->
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

      <!-- Text Input -->
      <input
        id="search-input"
        type="text"
        placeholder={getPlaceholderText()}
        class="value-input"
        class:error={parseError !== null}
        value={inputValue}
        oninput={handleInput}
        onkeydown={handleKeydown}
      />

      <!-- Search Button -->
      <button
        class="search-button"
        onclick={onSearch}
        aria-label="Search"
        disabled={!inputValue.trim()}
      >
        <svg
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
          <path d="m9 18l6-6l-6-6" />
        </svg>
      </button>
    </div>

    <!-- Parse Error Display -->
    {#if parseError}
      <div class="error-message">
        {parseError}
      </div>
    {/if}

    <!-- Field Suggestions Popover -->
    <ul
      id="field-suggestions-popover"
      class="popover-base suggestions-popover"
      popover="auto"
    >
      {#each fieldSuggestions as field, index}
        <li>
          <button
            class="list-button"
            class:selected={index === selectedSuggestionIndex}
            onclick={() => selectField(field)}
          >
            <span class="field-name">{field.name}</span>
            <span class="field-description">{field.description}</span>
          </button>
        </li>
      {/each}
    </ul>
  </div>
</div>

<style>
  /* Search Container */
  .search-container {
    @apply flex items-center gap-2;
    @apply input input-bordered input-sm;
    @apply w-full px-2 py-1;
  }

  .search-icon {
    @apply w-4 h-4 flex-shrink-0 text-base-content/60;
  }

  /* Text Input */
  .value-input {
    @apply flex-1 bg-transparent;
    @apply border-none outline-none;
    @apply text-sm min-w-0;
    @apply placeholder:text-base-content/40;
  }

  .value-input.error {
    @apply text-error;
  }

  /* Search Button */
  .search-button {
    @apply flex items-center justify-center;
    @apply w-6 h-6;
    @apply text-base-content/60;
    @apply transition-colors;
    @apply hover:text-primary;
    @apply disabled:opacity-40 disabled:cursor-not-allowed;
  }

  /* Error Message */
  .error-message {
    @apply absolute top-full left-0 right-0;
    @apply mt-1 px-3 py-2;
    @apply bg-error/10 text-error;
    @apply text-xs rounded-md;
    @apply border border-error/20;
  }

  /* Suggestions Popover */
  .popover-base {
    @apply bg-base-100 rounded-md shadow-lg;
    @apply border border-base-300;
    @apply p-0 mx-0 my-2;
    @apply min-w-60 max-w-96;
  }

  .suggestions-popover {
    position-anchor: --search-anchor;
    top: anchor(--search-anchor bottom);
    left: anchor(--search-anchor left);
  }

  /* Suggestion List Items */
  .list-button {
    @apply w-full px-3 py-2;
    @apply flex flex-col items-start gap-0.5;
    @apply text-left;
    @apply transition-colors;
    @apply hover:bg-base-200;
    @apply border-none bg-transparent;
    @apply cursor-pointer;
  }

  .field-name {
    @apply text-sm font-medium text-base-content;
  }

  .field-description {
    @apply text-xs text-base-content/60;
  }

  .list-button.selected {
    @apply bg-primary text-primary-content;
  }

  .list-button.selected .field-name {
    @apply text-primary-content;
  }

  .list-button.selected .field-description {
    @apply text-primary-content/80;
  }
</style>

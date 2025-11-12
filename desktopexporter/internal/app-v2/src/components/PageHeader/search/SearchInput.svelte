<script lang="ts">
  import {
    getFieldsBySignal,
    getDynamicAttributes,
    type FieldDefinition,
  } from '@/constants/fields';
  import { type QueryNode } from './queryTree';
  import { parseQuery } from './queryParser';
  import { telemetryAPI } from '@/services/telemetry-service';
  import { getTimeContext } from '@/contexts/time-context.svelte';

  // Component Props
  let {
    signal,
    view,
    placeholder,
  }: {
    signal: 'traces' | 'logs' | 'metrics';
    view: 'list' | 'detail';
    placeholder?: string;
  } = $props();

  // Get time context during component initialization
  let timeContext: any = null;
  try {
    timeContext = getTimeContext();
  } catch (error) {
    console.warn('Could not get time context during initialization:', error);
  }

  const fields = [
    ...getFieldsBySignal(signal),
  ];
  let availableFields = $state<FieldDefinition[]>([...fields]);

  // Load dynamic attributes
  async function loadDynamicAttributes() {
    if (timeContext) {
      let dynamicAttrs: FieldDefinition[] = await getDynamicAttributes(
        timeContext.selection.start, 
        timeContext.selection.end, 
        signal
      );
      availableFields = [...fields, ...dynamicAttrs];
    }
  }

  // Load on mount
  loadDynamicAttributes();
  // Core State
  let inputValue = $state(''); // Text input value
  let queryTree = $state<QueryNode | null>(null); // Parsed query tree
  let parseError = $state<string | null>(null); // Parser error message

  // Autocomplete State
  let fieldSuggestions = $state<FieldDefinition[]>([]);
  let showSuggestions = $state(false);
  let selectedSuggestionIndex = $state(0);
  let suggestionsDebounceTimer: ReturnType<typeof setTimeout> | null = null;


  



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
      detectedPatternValue = null; // Clear pattern when input is too short
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

    // No pattern detected - clear any stored pattern value
    if (detectedPatternValue) {
      detectedPatternValue = null;
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

  // Clear all suggestion state
  function clearSuggestions() {
    showSuggestions = false;
    fieldSuggestions = [];
    selectedSuggestionIndex = 0;
    detectedPatternValue = null;
  }

  // Detect Pattern Fields (Trace ID, Span ID)
  let detectedPatternValue = $state<string | null>(null);

  // Check If Pattern Is Already Part Of A Field Expression
  function isPatternInFieldExpression(input: string, matchIndex: number): boolean {
    // Get text before the match
    const beforeMatch = input.substring(0, matchIndex);
    
    // Check if there's a field name and = operator before the pattern
    // Look for pattern: fieldName= or fieldName = (with optional spaces)
    const fieldExpressionPattern = /([\w.]+)\s*=\s*$/;
    return fieldExpressionPattern.test(beforeMatch);
  }

  function detectPatternFields(input: string): FieldDefinition[] {
    const traceIdPattern = /\b([a-f0-9]{32})\b/i;
    const spanIdPattern = /\b([a-f0-9]{16})\b/i;

    // Check for trace ID
    const traceIdMatch = input.match(traceIdPattern);
    if (traceIdMatch) {
      const matchIndex = traceIdMatch.index!;
      // Don't suggest if pattern is already part of a field expression
      if (!isPatternInFieldExpression(input, matchIndex)) {
        detectedPatternValue = traceIdMatch[1];
        return fuzzyMatchFields('traceId');
      }
    }

    // Check for span ID
    const spanIdMatch = input.match(spanIdPattern);
    if (spanIdMatch) {
      const matchIndex = spanIdMatch.index!;
      // Don't suggest if pattern is already part of a field expression
      if (!isPatternInFieldExpression(input, matchIndex)) {
        detectedPatternValue = spanIdMatch[1];
        return fuzzyMatchFields('spanId');
      }
    }

    // No pattern detected
    detectedPatternValue = null;
    return [];
  }

  // Fuzzy Match Field Names
  function fuzzyMatchFields(input: string): FieldDefinition[] {
    const matches = availableFields
      .filter(field => field.searchScope !== 'global')
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
    if (field.searchScope === 'global') return 0;
    const lowerInput = input.toLowerCase();
    const lowerFieldName = field.name.toLowerCase();

    // Exact match gets highest score
    if (lowerFieldName === lowerInput) return 10;

    // Prefix match gets high score
    if (lowerFieldName.startsWith(lowerInput)) return 8;

    // Contains match gets medium score
    if (lowerFieldName.includes(lowerInput)) return 5;

    return 0;
  }

  // Select A Field From Suggestions
  function selectField(field: FieldDefinition) {
    if (field.searchScope === 'global') return;
    let newValue: string;

    if (detectedPatternValue) {
      // Pattern mode: replace the specific detected value with field=value
      newValue = inputValue.replace(
        detectedPatternValue,
        field.name + '=' + detectedPatternValue
      );
    } else {
      // Normal mode: standard field insertion
      const context = analyzeTypingContext(inputValue);

      if (context.partial) {
        // Replace the partial field name
        const beforePartial = inputValue.substring(
          0,
          inputValue.length - context.partial.length
        );
        newValue = beforePartial + field.name + '=';
      } else {
        // Append field name
        newValue = inputValue + field.name + '=';
      }
    }

    // Update input value
    inputValue = newValue;

    // Clear all suggestion state
    clearSuggestions(); // Clear the stored pattern value

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

        // Get time context for filtering by time range, with fallback
        let startTime = 0;
        let endTime = Date.now();

        if (timeContext) {
          if (timeContext.selection.type === 'preset') {
            // For presets, calculate fresh time range based on current time
            const duration =
              timeContext.selection.end - timeContext.selection.start;
            startTime = endTime - duration;
          } else {
            // For custom/recent selections, use the stored values as-is
            startTime = timeContext.selection.start;
            endTime = timeContext.selection.end;
          }
        } else {
          console.warn(
            'No time context available, using fallback range (0 to now)'
          );
          // Fallback to all traces (0 to now)
        }

        // Call the appropriate API method based on signal type
        let searchPromise: Promise<any>;

        switch (signal) {
          case 'traces':
            if (view === 'list') {
              searchPromise = telemetryAPI.searchTraces(
                startTime,
                endTime,
                queryTree
              );
            } else {
              // TODO: For detail view, we'll need a traceID prop to search within a specific trace
              // searchPromise = telemetryAPI.searchSpansInTrace(traceID, startTime, endTime, queryTree);
              console.warn('Detail view search not yet implemented');
              return;
            }
            break;
          case 'logs':
            // TODO: Implement logs search
            console.warn('Logs search not yet implemented');
            return;
          case 'metrics':
            // TODO: Implement metrics search
            console.warn('Metrics search not yet implemented');
            return;
          default:
            throw new Error(`Unknown signal type: ${signal}`);
        }

        searchPromise
          .then(results => {
            console.log('Search results:', results);
            // TODO: Handle results - emit event or update parent state
            // This depends on how you want to integrate with the rest of your app
          })
          .catch(error => {
            parseError = 'Search failed: ' + error.message;
          });
      }
    } catch (error) {
      parseError = error instanceof Error ? error.message : 'Parse error';
    }
  }

  // Generate Placeholder Text
  function getPlaceholderText(): string {
    if (placeholder) return placeholder;
    if (signal) return `Search ${signal}...`;
    return 'Search...';
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
            <span class="field-name">{'name' in field ? field.name : ''}</span>
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

  .list-button.selected {
    @apply bg-primary text-primary-content;
  }

  .list-button.selected .field-name {
    @apply text-primary-content;
  }
</style>

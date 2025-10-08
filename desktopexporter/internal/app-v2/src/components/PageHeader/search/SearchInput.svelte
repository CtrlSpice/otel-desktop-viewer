<script lang="ts">
  import { getFieldSuggestions, type FieldDefinition } from './search-fields';

  interface ParsedQuery {
    type: 'field' | 'attribute' | 'text';
    field?: string;
    operator?: '=' | '!=' | '~' | '>' | '<' | '>=' | '<=';
    value?: string | number | boolean;
    isValid: boolean;
  }

  let {
    signal,
    view,
    placeholder,
    onSearch = null,
  }: {
    signal: 'traces' | 'logs' | 'metrics';
    view: 'list' | 'detail';
    placeholder?: string;
    onSearch?: ((value: string) => void) | null;
  } = $props();

  let searchValue = $state('');
  let showSuggestions = $state(false);
  let suggestions = $state<FieldDefinition[]>([]);
  let debounceTimer: ReturnType<typeof setTimeout> | null = null;

  // Get available fields once at component creation
  const availableSuggestions = getFieldSuggestions(signal, view);

  // Effect to show/hide popover when showSuggestions changes
  $effect(() => {
    const popover = document.getElementById('suggestions-popover');
    if (popover) {
      if (showSuggestions && suggestions.length > 0) {
        popover.showPopover();
      } else {
        popover.hidePopover();
      }
    }
  });

  // Generate placeholder text based on signal
  function getPlaceholderText(): string {
    if (placeholder) return placeholder;
    return `Search ${signal}...`;
  }

  function handleInput(event: Event) {
    const target = event.target as HTMLInputElement;
    searchValue = target.value;

    // Clear existing debounce timer
    if (debounceTimer) {
      clearTimeout(debounceTimer);
    }

    // Debounce the suggestion logic
    debounceTimer = setTimeout(() => {
      // Early exit for very short queries
      if (searchValue.length < 2) {
        showSuggestions = false;
        suggestions = [];
        return;
      }

      // Get suggestions (pattern detection + fuzzy matching)
      let newSuggestions = getSuggestions(searchValue);
      if (newSuggestions.length > 0) {
        suggestions = newSuggestions;
        showSuggestions = true;
      } else {
        showSuggestions = false;
        suggestions = [];
      }
    }, 150); // 150ms debounce

    // Always call onSearch immediately (no debounce for search)
    if (onSearch) {
      onSearch(searchValue);
    }
  }

  function acceptSuggestion(suggestion: FieldDefinition) {
    searchValue = suggestion.suggestion;
    showSuggestions = false;
    if (onSearch) {
      onSearch(searchValue);
    }
  }

  // Main function to get all suggestions
  function getSuggestions(input: string): FieldDefinition[] {
    // First check for pattern detection (like traceId, spanId)
    const detectedPatterns = detectPattern(input);
    if (detectedPatterns.length > 0) {
      return detectedPatterns;
    }

    // If no pattern detected, try fuzzy matching on field names
    return getFuzzyFieldSuggestions(input);
  }

  // Pattern detection logic
  function detectPattern(input: string): FieldDefinition[] {
    const traceIdPattern = /^[a-f0-9]{32}$/i; //32 hex characters
    const spanIdPattern = /^[a-f0-9]{16}$/i; // Span ID pattern: 16 hex characters

    if (traceIdPattern.test(input)) {
      let suggestions = getFuzzyFieldSuggestions('traceId').map(field => ({
        ...field,
        suggestion: field.suggestion + ' ' + field.operators[0] + ' ' + input,
      }));
      return suggestions;
    }

    if (spanIdPattern.test(input)) {
      // Get all fields that include "span" in their name
      let suggestions = getFuzzyFieldSuggestions('spanId').map(field => ({
        ...field,
        suggestion: field.suggestion + ' ' + field.operators[0] + ' ' + input,
      }));
      return suggestions;
    }

    return [];
  }

  // Get fuzzy matched field suggestions
  function getFuzzyFieldSuggestions(query: string): FieldDefinition[] {
    // Apply fuzzy matching
    const matches = availableSuggestions
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
      .slice(0, 10); // Limit to 10 suggestions

    return matches.map(match => match.field);
  }
</script>

<div class="form-control flex-1">
  <div class="relative" style="anchor-name: --search-anchor">
    <svg
      class="absolute left-3 top-1/2 transform -translate-y-1/2 w-4 h-4"
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
    <input
      id="search-input"
      type="text"
      placeholder={getPlaceholderText()}
      class="input input-bordered input-sm pl-10 w-full"
      value={searchValue}
      oninput={handleInput}
    />

    <ul id="suggestions-popover" class="suggestions-popover" popover="hint">
      {#each suggestions as field, index}
        <li>
          <button
            class="list-button group"
            onclick={() => acceptSuggestion(field)}
          >
            <div class="flex-1 min-w-0">
              <div class="text-base-content/90 font-medium">
                {field.suggestion}
              </div>
              {#if field.description}
                <div class="description-slide">
                  {field.description}
                </div>
              {/if}
            </div>
          </button>
        </li>
        {#if index < suggestions.length - 1}
          <div class="border-t border-base-300"></div>
        {/if}
      {/each}
    </ul>
  </div>
</div>

<style>
  .suggestions-popover {
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
</style>

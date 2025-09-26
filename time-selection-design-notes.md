# Time Selection Functionality Design Decisions

## 1. Time Range State Management

- **Decision**: DateTimeFilter manages its own state and communicates changes up
- **Reasoning**: Different pages (traces/metrics/logs) will handle time changes differently (display timestamps, select data, etc.)
- **Implementation**: DateTimeFilter maintains internal state, dispatches events to parent components

## 2. Preset Selection

- **Decision**: Immediately apply filter (A) + update button text (C)
- **Implementation**:
  - DateTimeFilter sets start/end time and fires change event
  - PageHeader receives event and updates button text
  - Drawer closes after selection
- **Preset Retention**: Need to track which preset was selected for display purposes
- **Cross-Signal Coordination**: Time selection is coordinated across all signals (Traces/Metrics/Logs)
- **Reasoning**: Users typically want same time period when investigating issues across different signal types
- **Persistence**: Time selection persisted in localStorage across sessions
- **Recently Used**: Captures actual time ranges (not relative presets) for reuse
- **Example**: "Last 30 minutes" at 2pm becomes "1:30pm - 2:00pm" in recents
- **State Tracking**: Need to track selection type (preset/custom/recent) for proper display
- **Storage Format**: Normalize to Unix timestamps (milliseconds) for consistency
- **Benefits**: Avoids timezone confusion, easier calculations, universal format

## Implementation Plan ✅ COMPLETED

### ✅ Step 1: Create Reactive Context (Replaced Time Store)

- Created `src/contexts/time-context.svelte.ts` with Svelte 5 runes
- Uses `$state` for reactive time selection and timezone
- Stores current selection as Unix timestamps with discriminated union types
- Persists to localStorage on every change
- **Key Innovation**: Context object uses getters to maintain reactivity across components

### ✅ Step 2: Update DateTimeFilter

- Replaced `onOpen()` function with reactive `$effect()`
- Uses context as single source of truth for display
- Maintains local draft state only for custom input fields
- Automatically syncs with context changes via reactive updates
- Eliminates intermediate state variables

### ✅ Step 3: Update PageHeader

- Uses `$derived(timeContext.selection)` to track context changes
- Auto-closes drawer when meaningful selections are made
- Updates button text reactively based on context
- Proper timezone change handling

### ✅ Step 4: Recently Used Implementation

- Integrated with DateTimeFilter component
- Stores in localStorage with 10-item limit
- Automatic deduplication and sorting by most recent
- Shows checkmark for current recent selection

### ✅ Step 5: Context Reactivity Solution

- **Problem**: Context object wasn't reactive across component boundaries
- **Solution**: Used getter functions in context object:
  ```typescript
  let contextObject = {
    get selection() { return selection; },
    get timezone() { return timezone; },
    setSelection,
    setTimezone,
  };
  ```
- **Result**: Components can use `$derived(timeContext.selection)` for proper reactivity

## TypeScript Syntax Explanation

### Intersection Types with Discriminated Unions

```typescript
// Base interface with common fields
interface BaseTimeSelection {
  start: number
  end: number
  displayText: string
}

// Type-specific extensions using discriminated unions
type TimeSelection = BaseTimeSelection &
  (
    | { type: "preset"; presetId: PresetId }
    | { type: "custom" }
    | { type: "recent"; recentEntryIndex: number }
  )
```

### Syntax Breakdown

#### 1. Base Interface

```typescript
interface BaseTimeSelection {
  start: number
  end: number
  displayText: string
}
```

Regular interface with common fields that all time selections share.

#### 2. Intersection Type with Discriminated Union

```typescript
type TimeSelection = BaseTimeSelection &
  (
    | { type: "preset"; presetId: PresetId }
    | { type: "custom" }
    | { type: "recent"; recentEntryIndex: number }
  )
```

**`BaseTimeSelection &`**

- The `&` is the **intersection operator**
- It combines `BaseTimeSelection` with whatever comes after
- Result: All fields from `BaseTimeSelection` PLUS the fields from the union

**`( ... )`**

- Parentheses group the union type
- This is just for clarity/readability

**`| { type: 'preset'; presetId: PresetId }`**

- The `|` is the **union operator** (means "OR")
- This creates a type that has `type: 'preset'` AND `presetId: PresetId`

**`| { type: 'custom' }`**

- Another union option
- Has `type: 'custom'` but no extra fields

**`| { type: 'recent'; recentEntryIndex: number }`**

- Third union option
- Has `type: 'recent'` AND `recentEntryIndex: number`

#### 3. How It Works Together

The intersection (`&`) combines the base interface with the union:

```typescript
// This creates a type that has:
// - All fields from BaseTimeSelection (start, end, displayText)
// - PLUS one of the union options based on the type field

// So the final type is equivalent to:
type TimeSelection =
  | {
      start: number
      end: number
      displayText: string
      type: "preset"
      presetId: PresetId
    }
  | { start: number; end: number; displayText: string; type: "custom" }
  | {
      start: number
      end: number
      displayText: string
      type: "recent"
      recentEntryIndex: number
    }
```

#### 4. Why This Syntax?

- **`&` (intersection)**: "Combine these types together"
- **`|` (union)**: "This OR that OR the other"
- **Parentheses**: Group the union so the intersection applies to the whole group

#### 5. Benefits

- **No repetition** of `start`, `end`, `displayText`
- **Clear separation** between common and type-specific fields
- **Easier to maintain** - add common fields in one place
- **Type safety** - TypeScript knows exact shape based on type field

## 3. Custom Time Range

- **Status**: Pending discussion

## 4. Recently Used

- **Status**: Pending discussion

## 5. Button Display

- **Status**: Pending discussion

## 6. Desktop Layout Considerations for DateTimeFilter

### Current Layout
- **Desktop Application**: Three-column horizontal layout optimized for desktop use
  - Left: Preset time ranges (Last 5 minutes, Last hour, etc.)
  - Middle: Custom time range inputs (start/end text fields)
  - Right: Recently used time ranges
  - Bottom: Timezone selector

### Desktop-Specific Considerations
- **Window Resizing**: Handle different desktop window sizes gracefully
- **Keyboard Navigation**: Ensure full keyboard accessibility for desktop users
- **Mouse Interactions**: Optimize hover states and click targets for mouse use
- **Screen Real Estate**: Make efficient use of available desktop space

### Layout Optimization
- **Minimum Width**: Ensure component works well in smaller desktop windows
- **Flexible Sizing**: Allow sections to resize proportionally
- **Overflow Handling**: Gracefully handle content that exceeds available space
- **Focus Management**: Proper focus handling for keyboard users

### Implementation Notes
- Focus on desktop-appropriate responsive breakpoints (e.g., `lg:`, `xl:`)
- Ensure adequate spacing and touch targets for desktop interaction
- Consider multi-monitor setups and various desktop resolutions

## Next Steps

Continue discussing remaining aspects before implementation.

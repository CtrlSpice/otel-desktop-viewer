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

## Implementation Plan

### Step 1: Create Time Store

- Create `src/stores/time-store.ts` with Svelte store
- Store current time selection as Unix timestamps
- Store recent entries array (max 10, sorted by most recent)
- Persist to localStorage with debounced writes

### Step 2: Update DateTimeFilter

- Remove internal state management
- Use time store for current selection
- Dispatch events for preset/custom/recent selections
- Update display text to use store data

### Step 3: Update PageHeader

- Listen to time store changes
- Update button text reactively
- Handle timezone changes

### Step 4: Update Pages

- TracesPage, MetricsPage, LogsPage use time store
- Remove individual time state management
- Coordinate time across all signals

### Step 5: Recently Used Implementation

- Add recent entries to time store
- Populate "Recently used" section
- Handle deduplication and limits

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

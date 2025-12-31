# Layout & display components

Components for organizing and displaying content.

## QueuedSticky

A system for managing "sticky" elements that should stack without overlapping,
particularly when nested in complex structures.

### Usage

```tsx
import {
  QueuedStickyScrollingBase,
  Sticky,
  StickyOffset,
} from '@/generic_libs/components/queued_sticky';

function Page() {
  return (
    <QueuedStickyScrollingBase>
      <Sticky top>
        <div>Sticky Header 1</div>
      </Sticky>
      <div>Content</div>
      <StickyOffset>
        <Sticky top>
          <div>Sticky Header 2 (Stacks below Header 1)</div>
        </Sticky>
        <div>Nested Content</div>
      </StickyOffset>
    </QueuedStickyScrollingBase>
  );
}
```

### Components

*   **`QueuedStickyScrollingBase`**: established the stacking context.
*   **`Sticky`**: The element that sticks. Accepts `top` or `bottom` props.
*   **`StickyOffset`**: Wraps content that comes *after* a sticky element
    effectively. `Sticky` elements inside `StickyOffset` will acknowledge the
    space taken by `Sticky` elements in parent contexts (siblings of the
    `StickyOffset`'s parent).

### Internal workings

It uses `ResizeObserver` to track dimensions of sticky elements and CSS
variables (`--accumulated-top`, etc.) to pass offsets down the tree. This
ensures that a nested sticky element knows exactly how much offset it needs to
avoid overlapping parent/sibling sticky headers.

---

## ExpandableEntry

A standard customized expandable section component, often used for logs or step
details.

### Usage

```tsx
import {
  ExpandableEntry,
  ExpandableEntryBody,
  ExpandableEntryHeader,
} from '@/generic_libs/components/expandable_entry';

<ExpandableEntry expanded={isExpanded}>
  <ExpandableEntryHeader onToggle={setIsExpanded}>
    Title
  </ExpandableEntryHeader>
  <ExpandableEntryBody>
    Hidden Content
  </ExpandableEntryBody>
</ExpandableEntry>
```

### Features

*   **Context-based State**: Uses `ExpandedContext` to coordinate between
    Header and Body (though state is lifted up).
*   **Mounting Strategies**:
    *   `renderChildren='was-expanded'`: (Default) Renders content once
        expanded, keeps it in DOM after collapse. Good for performance.
    *   `renderChildren='always'`: Always mounted.
    *   `renderChildren='expanded'`: Unmounts when collapsed.

---

## TreeTable

A table component that supports hierarchical data (expandable rows).

### Usage

```tsx
import { TreeTable, RowData } from '@/generic_libs/components/table';

const data: RowData[] = [
  { id: '1', name: 'Root', children: [{ id: '1.1', name: 'Child' }] }
];

<TreeTable
  data={data}
  columns={[{ id: 'name', label: 'Name' }]}
  placeholder="No data"
  expandedRowIds={externalSet} // Optional: for controlled mode
  onExpandedRowIdsChange={setExternalSet}
/>
```

### Internal workings

*   **Recursion**: Flattens the hierarchical data into a list of `VisibleRow`s
    based on `expandedRowIds`.
*   **Indentation**: Adds padding to the first column based on row depth
    (`level * 24px`).
*   **Controlled/Uncontrolled**: Supports both modes for expansion state.

---

## HighlightedText

Simple component to bold a substring within text.

### Usage

```tsx
import { HighlightedText } from '@/generic_libs/components/highlighted_text';

// Renders "Hello <b>World</b>"
<HighlightedText text="Hello World" highlight="World" />
```

### Internal workings

Uses a standard RegExp with `escapeRegExp` to find the **first** occurrence
(case-insensitive) and splits the string into prefix, match, and suffix.

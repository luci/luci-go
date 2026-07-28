# Material-React-Table (MRT) Architecture

## Context
Fleet Console uses `Material-React-Table` (MRT) across Android, Browser, and ChromeOS device tables. This document defines our shared layout architecture.

## Decision
We combine the **Dumb Toolbar Pattern** with **Component Splitting** for all device tables.

### 1. Component Splitting
Page components (e.g., `chromeos_devices_page.tsx`) manage page state, data fetching, and table setup.
Extract custom toolbars (`renderTopToolbarCustomActions`, `renderBottomToolbarCustomActions`) into separate files in the same directory.

### 2. Dumb Toolbar Pattern
Toolbar components receive the `table` instance as a prop and read state through it rather than calling page hooks directly.

Pass page-specific callbacks via the `meta` option in `useFCDataTable`:

```typescript
const table = useFCDataTable({
  columns,
  data,
  meta: {
    customAction: () => { ... },
    pageSpecificData: data,
  },
});
```

Reading `meta` in toolbars:
```typescript
export function TopToolbarCustomActions<TData extends MRT_RowData>({ table }: { table: MRT_TableInstance<TData> }) {
  const meta = table.options.meta as {
    customAction: () => void;
  };
  // use meta.customAction
}
```

### 3. URL Synchronization & Filters
- **Use `useFilters`**: Synchronize filters with the URL using `useFilters` and its AIP-160 parser.
- **In-Column Filtering**: Intercept column filter changes using `onColumnFiltersChangeOverride` and sync them to `useFilters`. Use a `useRef` to store column filters and prevent update loops.
- **Key Translation**: Map backend dimension keys to canonical URL/Table keys at page initialization.

### 4. Fallback Options
When a column needs dropdown filter options missing from the backend API (e.g., `Realm`), extract unique values from visible table rows as fallbacks.

### 5. Centralized Column Definitions
- **Dynamic Generator Pattern**: Construct column lists dynamically using `getColumns(columnIds: string[])`.
- **Decouple Layout from Fields**: Place field accessors, headers, and custom cell renderers in domain config files (e.g., `src/fleet/config/fields/chromeos.tsx`).
- **Co-locate Configs**: Store field configs and layout definitions in the same directory as the page component.

### 6. Row Virtualization
Row virtualization is enabled by default in `useFCDataTable` to keep scrolling smooth at 1,000 rows. Virtualization is disabled in tests (`process.env.NODE_ENV === 'test'`) because JSDOM lacks a layout engine.

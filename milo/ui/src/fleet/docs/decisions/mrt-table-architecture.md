# Copyright 2026 The LUCI Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Decision: Material-React-Table (MRT) Architecture in Fleet Console

## Context
The Fleet Console is migrating its data tables from legacy MUI `DataGrid` to `Material-React-Table` (MRT). To ensure maintainability and consistency across different pages (Android, Browser, ChromeOS devices), we need a shared architecture for configuring tables and toolbars while allowing for platform-specific customizations.

## Decision
We adopt the **"Dumb" Toolbar Pattern** combined with **Component Splitting** for all major device list tables.

### 1. Component Splitting
For pages with complex tables, the main page file (e.g., `chromeos_devices_page.tsx`) should focus on page-level state, data fetching, and table configuration.
The custom toolbars (rendered via `renderTopToolbarCustomActions` and `renderBottomToolbarCustomActions`) should be extracted into separate files within the same directory.

### 2. The "Dumb" Toolbar Pattern
Extracted toolbar components should be "dumb" in the sense that they do not access page-level hooks or contexts directly. Instead, they should receive the `table` instance as a prop and access all necessary state and actions through it.

#### Accessing Custom State via `meta`
To pass page-specific data or callbacks to the toolbar without inflating the prop list, use the `meta` option in `useFCDataTable` (or the raw MRT options).

**Example: Providing Meta**
```typescript
const table = useFCDataTable({
  columns,
  data,
  meta: {
    customAction: () => { ... },
    pageSpecificData: data,
  },
  // ...
});
```

**Example: Using Meta in Toolbar**
```typescript
export function TopToolbarCustomActions<TData extends MRT_RowData>({ table }: { table: MRT_TableInstance<TData> }) {
  const meta = table.options.meta as {
    customAction: () => void;
    pageSpecificData: DataType;
  };
  // ... use meta.customAction ...
}
```

### 3. URL Synchronization and Filter Parsing
To avoid conflicts between different filter formats, we strictly use the new `useFilters` hook and its AIP-160 compliant parser for URL synchronization.
- **Avoid Legacy Parsers**: Do not use legacy parsers like `getFilters` in new pages, as they may reject valid AIP-160 operators like `:` or `!=`.
- **In-Column Filtering**: When enabling MRT in-column filtering, intercept changes using `onColumnFiltersChangeOverride` (or handling `onColumnFiltersChange` directly) and synchronize them with the `useFilters` hook. Use a `useRef` to store current column filters to safely prevent circular update loops during state synchronization.
- **Key Mapping Principle**: Avoid complex key mapping and regex replacements at the hook level. Instead, translate backend dimension keys to canonical URL/Table keys at the edge (e.g., during page-level initialization of filter builders) to ensure consistency across all systems.

### 4. Fallback Options for Missing Dimensions
If a column needs a dropdown filter but its available values are not returned by the backend dimensions API (e.g., `Realm`), extract the unique values from the currently visible rows in the table and inject them as fallback options.

### 5. Column Configuration Patterns
To ensure column configurations are centralized, easy to edit, and maintain a clear separation of concerns:

- **Standardize on the Dynamic Generator Pattern:** All tables should use a generator function pattern (e.g., `getColumns(columnIds: string[])`) to construct the column list dynamically. This provides a consistent interface for column management hooks, even for tables with a fixed set of columns.
- **Decouple Field Configs from Layout:** All field-specific data accessors, headers, and custom cell renderers (overrides) must be moved out of the table-layout columns file and placed into a dedicated domain configuration file (e.g., `src/fleet/config/fields/chromeos.tsx`).
- **Declarative Column Definitions:** The columns file should only describe the *layout* and *assembly* of columns, mapping the requested IDs to the field overrides.

#### File Organization Plan
To ensure high locality and make it intuitive for developers:
- **Co-locate Configurations and Layouts:** Both field configurations (data accessors, fallbacks) and table layout definitions should live in the **same directory as the page component** that uses them (e.g., `src/fleet/pages/device_list_page/chromeos/`).

### 6. Virtualization for Large Datasets
To prevent UI sluggishness when displaying large datasets (e.g., up to 1,000 rows per page), row virtualization is enabled by default in the centralized hook.
- **Why**: Rendering 1,000 rows without virtualization floods the DOM, causing slow column resizing and heavy load times.
- **UX Impact**: Virtualization requires a scroll container with a defined height, which automatically makes the table header sticky at the top of the container. In tests, row virtualization is disabled (via `process.env.NODE_ENV !== 'test'`) to prevent failures related to JSDOM's lack of layout engine.

## Benefits
- **Maintainability**: Main files are smaller and easier to read.
- **Consistency**: All tables follow the same structural pattern.
- **Customization**: Each page can define its own toolbar component with specific buttons, while sharing the core table setup.
- **Testability**: Dumb toolbars are easier to unit test by mocking the `table` instance.
- **Robustness**: Centralized URL state management prevents conflicting UI states.
- **Performance**: Virtualization provides smooth scrolling and interaction (like resizing) even at 1,000 rows.

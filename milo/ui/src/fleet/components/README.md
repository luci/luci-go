# Fleet Console Shared UI Components & Utilities

This directory (`src/fleet/components/`) contains reusable React UI components shared across multiple Fleet Console pages and features, paired with pure helper utilities in [`../utils/`](../utils/).

## Discovery & Reuse Convention

Rather than maintaining an exhaustive static list of every component, shared code is organized into self-descriptive **domain subdirectories**:

1. **Scan Before Building**: Before creating a new UI widget, dialog section, table cell, tooltip, or text formatter, list or search `src/fleet/components/` and `src/fleet/utils/` to check if a component or helper already exists for that domain.
2. **Extract When Shared Across Features**: If a component or utility is needed in more than one feature or page (for example, both UFS inventory dialogs in `src/fleet/pages/` and device actions in `src/fleet/components/actions/`), place or extract it into `src/fleet/components/<domain>/` or `src/fleet/utils/` instead of duplicating page-local code.
3. **Keep Pure Logic in `.ts` Utilities**: To comply with React Fast Refresh (`react-refresh/only-export-components`), `.tsx` files should only export React components; place pure formatting, parsing, and URL helpers in `src/fleet/utils/` (or a sibling `<feature>_utils.ts` file).

## Key Domain Directories

- **`code_snippet/`**: Copyable monospace and Markdown blocks (`<CodeSnippet />` for CLI commands/snippets and `<MarkdownSnippet />` for inline or collapsible Buganizer Markdown previews).
- **`device_table/` & `fc_data_table/`**: Shared Material-React-Table (`MRT`) wrappers and column management.
- **`filters/` & `filter_dropdown/`**: Standardized AIP-160 filter bar and category dropdown components.
- **`ellipsis_tooltip/` & `info_tooltip/`**: Truncated text tooltips (`<EllipsisTooltip />`) and help/info icon tooltips (`<InfoTooltip />`).
- **`options_dropdown/` & `segmented_toggle/`**: Reusable action menus and segmented view toggles.
- **`summary_header/`**: Page and section summary metric headers.
- **`actions/`**: Shared device management action flows (e.g., `autorepair/`, `ssh/`, `copy/`).
- **`../utils/`**: Shared pure utilities, including `markdown_utils.ts` (`md`, `rawMd`, `toFullUrl`), `aip160/` filter builders, and `dates.ts`.

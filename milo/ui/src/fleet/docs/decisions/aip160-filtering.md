# AIP-160 Filtering Architecture

## Context
Fleet Console uses a TypeScript port of our backend Go AIP-160 parser (`parser.ts`) to parse raw search expressions directly on the frontend.

## Architecture

### 1. Ported AST Parser
The parser in `utils/aip160/parser/parser.ts` mirrors the Go backend implementation to ensure exact parity:
- Follows the EBNF grammar for AIP-160.
- Converts search strings into Abstract Syntax Tree (AST) nodes instead of running fragile regexes.

### 2. Reading & Writing
- **Parser**: Reads filter strings and validates syntax, supporting operators like `:` and `!=`.
- **Serializer**: `serializer.ts` writes AST nodes back to URL strings using parenthesized OR groups for backward compatibility.

## Transition Rules

- **Multiple Values**: Use `key = v1 OR key = v2` as the primary format. Grouped syntax `key = (v1 OR v2)` is supported for backward compatibility.
- **Exclusions**: Use `label != A AND label != B`.
- **Blank Searches**: Use `NOT key:*`. Code generates `NOT key` during the transition phase.

### Backwards Compatibility
URL compatibility is best-effort. In the future, versioned URL translation functions will map legacy query strings to modern formats.

## Specific Caveats & Edge Cases

### Mismatched Parentheses
AIP-160 allows choices written as `key = v1 OR key = v2` or `key = (v1 OR v2)`. Legacy Fleet Console systems used parenthesized grouping. Consumers of our AST MUST analyze arguments recursively to avoid missing options wrapped inside parens.

### Deep Loop Trap in React Hooks
Updating filter state inside `onColumnFiltersChange` can trigger infinite re-render loops if state references mutate directly. Always use a `useRef` intercept to stabilize callbacks.

## Child Component Guidelines

1. **Parent Manages State**: Parent pages (`AndroidDevicesPage`, `ChromeOSDevicesPage`) own filter state via `useFilters`.
2. **Prop Passing**: Pass `aip160: string` and `setFiltersBatch` to child components. Child components do not read the URL directly.
3. **Data Queries**: Query hooks use the `aip160` string directly to stay in sync with filter bars.
4. **Click Actions**: Call `setFiltersBatch` with target updates (or empty arrays to clear filters).

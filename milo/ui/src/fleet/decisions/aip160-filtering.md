# Decision: AIP-160 Filtering Architecture in Fleet Console

## Context
To support complex, rich search capabilities in the Fleet Console UI, we made the decision to port an existing backend Go parser for AIP-160 (API Filtering) into TypeScript. This allows us to parse raw logical expressions directly on the frontend and map them directly to our Material-React-Table (MRT) and custom hook states.

While the parser provides a massive step forward, the overall migration path involves handling dual standards between legacy URL systems and the strict AST-based rules of the new parser.

## Our Approach to AIP-160

### 1. Ported Parser Architecture
The new parser in [parser.ts](../utils/aip160/parser/parser.ts) was ported directly from our Go backend implementation to ensure exact parity between what the UI sees and what the backend expects.
*   It follows the strict EBNF grammar for AIP-160 (omitting custom function call support for simplicity in this initial version).
*   It operates on full Abstract Syntax Tree (AST) representations, breaking strings like `key = v1 OR key = v2` into nodes rather than handling strings via fragile regexes.

### 2. Reading vs Writing Split
The migration strategy in Fleet Console prioritized the **reading side** (string to AST) to ensure we could understand complex queries first.
*   **Parser:** Strictly reads strings and validates correctness on load, supporting advanced operators like `:` and `!=`.
*   **Serializer Status:** A serializer exists in `serializer.ts` to handle the writing side. It currently generates parenthesized OR groups and uses legacy blank checks to maintain backward compatibility during this transition phase.

## Planned Transition to AIP-160

The migration is structured as a phased transition to full AIP-160 compliance. The current implementation represents an intentional intermediate state to ensure safety and compatibility while extending capabilities:

*   **Multiple Values:**
    *   *Target Intent:* Use `key = value1 OR key = value2`. This is the preferred syntax moving forward.
    *   *Current Transition State:* The grouped syntax `key = (value1 OR value2)` is kept for backwards compatibility reasons.
*   **Exclude Options:**
    *   *Target Intent:* Use `label != A AND label != B` (Option B).
    *   *Current Transition State:* The serializer does not generate compound boolean excludes yet.
*   **Blank Searches:**
    *   *Target Intent:* Use `NOT key:*` for blank searches (Option A).
    *   *Current Transition State:* Code generates `NOT key`, maintaining the legacy format.

The parser is capable of understanding the target formats, but the UI components and serializer remain in this intermediate phase for safety during the transition.

### Backwards Compatibility
Backwards compatibility for legacy URL formats is supported on a **best-effort basis** rather than as a strict guarantee. Supporting all past versions and their bugs indefinitely impacts development velocity.
In the future, a versioned URL system will be used with a dedicated migration function to translate old URLs. This avoids the need for parsers to support all historical formats. In rare occasions where a break occurs, users can re-add their filters.

### Future of useFilters
In the future, we want `useFilters` to be solely responsible for handling reading and writing to the `"filters"` URL param.

## Specific Caveats for Fleet Console

### Mismatched Parentheses
AIP-160 allows choices to be written as either `key = v1 OR key = v2` or `key = (v1 OR v2)`. The legacy system in Fleet Console used the parenthesized grouped format. To avoid missing options wrapped inside parens, consumers of our AST should recursively analyze arguments.

### Deep Loop Trap in React Hooks
MRT state hooks and `useFilters` hook both try to synchronize URL search states. Because of the sensitive shape difference between a raw array of string filters and custom types, standard equality checks like `_.isEqual` can sometimes throw false-positive update states, which may lead to infinite React depth loops.

***

**This document represents the active design guidelines for our AIP-160 migration stack. Please refer to this file when writing or debugging filter interactions.**

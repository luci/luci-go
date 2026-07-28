# URL Backwards Compatibility Principles

## Context
This document defines guidelines for URL backwards compatibility in Fleet Console.

## Principles

1. **Tiered Guarantees**:
   - **Top-Level Paths (Strong Guarantee)**: Primary page URLs must maintain working redirects or aliases.
   - **Complex State Parameters (Best Effort)**: Complex parameters like `filters` use best-effort parsing and fall back to default views if parsing fails.
2. **Focus on High-Usage Patterns**: Prioritize compatibility for default visible columns (e.g., `CHROMEOS_DEFAULT_COLUMNS`) and common filter fields rather than obscure edge cases.
3. **Centralized Rules**: Handle broad label prefix updates (e.g., `swarming_labels.` to `sw.`) using central translation rules rather than ad-hoc component logic.
4. **Graceful Recovery**: If URL parameters fail to parse, apply valid sub-filters, fall back safely, and display a user warning. Do not crash.
5. **Self-Healing URLs**: When translating a legacy URL, update the browser URL bar to the modern syntax so new bookmarks use the clean format.
6. **Time-Boxed Support**: Set deprecation windows (e.g., 6 months) for legacy URL translation rules.
7. **Observability**: Log telemetry when legacy URL rules trigger or fail to track usage before removing translation code.

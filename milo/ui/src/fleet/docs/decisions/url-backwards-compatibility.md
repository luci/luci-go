# Decision: URL Backwards Compatibility Principles in Fleet Console

## Context
As the Fleet Console evolves, we frequently need to update URL structures, path mappings, and complex query parameters (like AIP-160 filters). While maintaining perfect backwards compatibility for all historical URLs is desirable, doing so indefinitely imposes a high maintenance burden and risks growing code complexity out of control.

This document defines the principles for pragmatically approaching URL backwards compatibility to balance developer velocity with user experience.

## Principles

### 1. Tiered Guarantees based on URL Type
*   **Top-Level Path URLs (Strong Guarantee):** High-level entry points and page URLs should almost always support redirects or aliases. Users expect these to work reliably.
*   **Complex State Params (Best Effort):** Parameters encoding complex UI state (like the `filters` param) are subject to "best effort" support. The system should try to parse them but may fall back to defaults if parsing becomes too complex or risky.

### 2. Focus on High-Usage Patterns
*   We should prioritize maintaining compatibility for the **most commonly used filters and parameters** rather than striving for 100% coverage of all edge cases.
*   **Proxy for Usage:** In the absence of detailed telemetry, a good proxy for identifying commonly used filters is to look at the **default columns explicitly configured in the app** (e.g., `CHROMEOS_DEFAULT_COLUMNS`). Fields that are visible by default are the most likely candidates for user-created filters and bookmarks.
*   Adoption and usage metrics of specific pages or features should guide the level of effort: lower adoption areas permit more aggressive breaking changes if it unblocks development.
*   **Coverage of Filter Types:** Where feasible, we should aim to ensure translations work across different filter types (e.g., range, multiselect) and for specific fields known to be affected by changes.

### 3. Broad Sweeping Translations for Systemic Changes
*   When making wide-sweeping changes (e.g., renaming label prefixes like `swarming_labels.` to `sw.`), we should implement broad translation rules in a centralized location rather than handling them ad-hoc in components.

### 4. Graceful Degradation
*   If a URL fails to parse or map correctly, the application **must not crash**.
*   **Recover as much as possible:** If only part of the filters are malformed, the rest of the filters should still be applied.
*   The UI should display a user-friendly warning explaining that some parts of the URL could not be loaded.

### 5. Self-Healing URLs
*   When a legacy URL is successfully translated, the application should update the browser's URL bar to the new format. This ensures that if the user bookmarks the page again, they will save the modern format.

### 6. Time-Boxed Support
*   Legacy URL translations should not live forever. We should define deprecation timelines (e.g., support for 6 months or until usage drops below a threshold) after which legacy translation code can be safely removed.

### 7. Observability
*   We should add telemetry to track when legacy URL translations are triggered and when they fail. This data will help us understand which legacy URLs are still active and when it is safe to remove support.

## Existing Patterns and Code to Leverage

*   **`rewriteLegacyKeys` in `search_param_utils.ts`:** We currently have a simple pattern for rewriting keys (e.g., mapping `swarming_labels.` to `sw.`). This should be formalized into the central URL migration function.
*   **Redirect Routes:** We use dedicated redirect components for handling legacy paths. This pattern should be continued for top-level URL guarantees.

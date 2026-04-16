# Bot Info UI Consolidation Design Decisions

## Context
User requested adding bot info section to Dimensions tab in device details page (ChromeOS, Chrome Browser).

## Current Architecture
We deprecated the separate "Bot info" tab and moved all the data to the "Dimensions" tab, as Bot dimensions were integrated with device dimensions.

## Design Tradeoffs Considered

### 1. Add bot info section to Dimensions tab while keeping Bot info tab
- **Pros:** Minimal change, preserves existing navigation.
- **Cons:** Redundant information across tabs, cluttered UI.

### 2. Deprecate Bot info tab and move all data to Dimensions tab
- **Pros:** Simplified UI, reduces tab clutter, aligns with integration of bot dimensions with device dimensions.
- **Cons:** Users need to adapt to finding bot info in the Dimensions tab.
- **Decision:** Chosen to improve overall UX and remove redundancy.

## Links
None.

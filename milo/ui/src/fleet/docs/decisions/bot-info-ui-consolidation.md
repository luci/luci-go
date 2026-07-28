# Bot Info UI Consolidation

## Context
Users requested bot information on the Dimensions tab of the device details page (ChromeOS and Chrome Browser).

## Architecture
We deprecated the separate "Bot info" tab and moved all bot dimensions into the "Dimensions" tab, unifying device and bot metadata in one view.

## Tradeoffs

### 1. Dual Tabs (Bot Info + Dimensions)
- **Pros:** Retains legacy tab navigation.
- **Cons:** Duplicates metadata across tabs and clutters the UI.

### 2. Consolidated Dimensions Tab
- **Pros:** Removes tab clutter, aligns with integrated bot/device dimensions, simplifies layout.
- **Cons:** Users must locate bot details within the Dimensions tab.
- **Decision:** We chose a single consolidated tab to remove visual clutter and streamline navigation.

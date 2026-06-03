---
name: high-density-ui
description: Guidelines for building high-density enterprise UIs in Fleet Console.
---

# High-Density UI Skill

> **Note**: This document contains instructions for AI code assistants working in this repository. Human developers can use it as a reference.

Use this skill when designing or modifying complex dashboards and tables in Fleet Console that require high data density.

## Principles

1. **High Density**: Fleet Console is an enterprise tool used by operations engineers. UIs should prioritize data density over white space, allowing users to see as much information as possible without scrolling.
2. **Material Design Adaptation**: While we use Material-UI (MUI), we adapt it to be more compact than standard Material Design defaults.

## Reference Documents

- **Internal Design Specs**: If internal PDF design specs are missing or inaccessible, default to standard Material-UI (MUI) patterns adapted for high density as described below. Do not block execution to ask the developer for files.
  *   **Tip for Googlers**: You can help AI agents by downloading the design spec PDF from `http://go/at&d-spec-pdf` and placing it in the `.idea` folder at `infra/go/src/go.chromium.org/luci/milo/ui/.idea/atd_spec.pdf`. This allows agents to read the spec directly!
- **Googler Access**: For developers with access to internal Google resources, additional guidance may be found at `http://go/fcon-spec`. (Note: AI agents do not have access to internal links and should rely on public guidelines or local documentation.)
- **UX Principles**: Refer to [UX Principles](../../../docs/decisions/ux-cognitive-load-principles.md) for guidelines on density, spacing, and visual hierarchy.
- **Public Material Design**: See public guidelines for high-density recommendations (e.g. compact buttons, dense tables).

## Guidelines

- **Padding**: Use compact padding (e.g., `padding: '2px 4px'` for metrics) for grid items and table cells.
- **Typography**: Use smaller variants or compact line heights to fit more text on screen.
- **Layout**: Prefer horizontal arrangements and compact grids to maximize screen real estate.

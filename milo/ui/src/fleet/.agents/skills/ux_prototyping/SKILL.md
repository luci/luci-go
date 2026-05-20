---
name: ux_prototyping
description: Guidelines for rapid prototyping and adhering to UX principles in Fleet Console.
---

# UX Prototyping Skill

> **Note**: This document contains instructions for AI code assistants working in this repository. Human developers can use it as a reference.

Use this skill when you need to create new UI views, modify existing layouts, or perform rapid prototyping for UX changes.

## Principles

1. **Adhere to UX Cognitive Load Principles**: Refer to `../../../docs/decisions/ux-cognitive-load-principles.md` for guidelines on density, spacing, and visual hierarchy.
2. **Use Material-UI (MUI)**: Leverage MUI components (Grid, Box, Typography, etc.) for layouts to ensure consistency.
3. **Mock Data for Prototyping**: When rapid prototyping, use mock data or hardcoded states to demonstrate the UI before hooking up real API calls.

## Procedures

### Rapid Prototyping
1. Create a new component or page.
2. Use hardcoded state or mock data to simulate loading and success states.
3. Present the mockup to the user for feedback before integrating with real queries.

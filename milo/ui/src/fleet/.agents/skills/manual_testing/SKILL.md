---
name: manual_testing
description: Instructions for manually testing the Fleet Console using the browser subagent.
---

# Manual Testing Skill

> **Note**: This document contains instructions for AI code assistants working in this repository. Human developers can use it as a reference.

Use this skill when you need to verify UI changes, filter interactions, or page loads in the actual browser.

## Prerequisites

1. The local dev server must be running (usually at `http://localhost:8080`).
2. The backend server must be running (usually at `localhost:8800`).

## Procedures

### Using Browser Execution
1. Use your available browser execution environments or subagents to navigate to the URL.
2. Provide a clear task, such as: "Navigate to `http://localhost:8080/ui/fleet/labs/p/chromeos/devices` and verify that the Device Health Metrics section is visible and rendered correctly."
3. Instruct the subagent to return a screenshot or DOM snippet to verify the state.

### Common Test Scenarios
- **Filter Verification**: Click a metric or select a filter in the dropdown and verify that the URL updates and the list refetches.
- **Responsive View**: Instruct the subagent to resize the window to mobile width and verify that borders and grids adapt correctly.

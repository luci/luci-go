---
name: manual-testing
description: Instructions for manually testing the Fleet Console using the browser subagent. Use when you need to verify UI changes, filter interactions, or page loads in the actual browser.
---

# Manual Testing Skill

> **Note**: This document contains instructions for AI code assistants working in this repository. Human developers can use it as a reference.

Use this skill when you need to verify UI changes, filter interactions, or page loads in the actual browser. For automated validations, see [project-verification](../project-verification/SKILL.md).

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

## Recent Findings & Best Practices

### Local Server Ports & CORS Issues
- **Finding**: Testing on `http://localhost:8001` (Vite preview) may encounter CORS errors when the frontend attempts to fetch data from the backend.
- **Action**: Use `http://localhost:8080/ui/fleet/p/android/devices?filters=` (the standard local server URL) instead, as it is properly configured to avoid CORS issues in the local dev environment.

### Automated Code Formatting
- **Finding**: Presubmit checks may fail due to Prettier/ESLint formatting errors.
- **Action**: Run `npx eslint --fix <file_path>` to automatically fix fixable errors before uploading the CL.

### High-Density UI Grid Alignment
- **Finding**: When moving or adding custom components to high-density grids (like `Device Health Metrics`), ensure horizontal padding and margins match existing components like `SingleMetric` to maintain vertical alignment.
- **Action**: Use `px: 1` (padding horizontal 1 unit) on containers to align with standard breakdown items, and `mt: 0.5` (margin top 0.5) to match spacing between headers and lists.

---
name: senior-reviewer
description: >-
  Performs a self-review of changed code using a Senior Reviewer subagent. Use
  before uploading a CL or completing a feature to identify bugs, security
  issues, styling drift, or style inconsistencies. Don't use for initial
  exploration, draft code spikes, or non-Fleet repositories.
---

# Senior Reviewer Skill

> **Note**: This document contains instructions for AI code assistants working in this repository. Human developers can use it as a reference.

Use this skill to perform a self-review of your changes using a subagent.

## Workflow

> [!IMPORTANT]
> **At the start of the self-review**, you MUST copy the progress checklist below into your very next response to the user, and check off the steps sequentially as you complete them.

Progress:
- [ ] Step 1: Invoke Reviewer Subagent (with full prompt + git diff)
- [ ] Step 2: Process Review Feedback
- [ ] Step 3: Iterate on Remediation (limit to 3 loops)

## Detailed Procedures

To perform the mandatory self-review, follow these steps:

1. **Invoke Subagent**: Use the `invoke_subagent` tool with `TypeName: "self"` and the prompt below.
2. **Pass Diff & Commit Message**: Include both the output of `git log -1 --format=%B` (commit message) and `git diff` directly inside the prompt along with the review instructions so the subagent audits both the code and the commit description / visual proof package.
3. **Address Feedback**: Iterate on the code and commit message until the reviewer subagent provides a clean review.
   - **Exit Condition**: Limit the cycle to a maximum of 3 iterations. If the reviewer still finds issues after 3 iterations, stop the loop, upload the CL, and flag the remaining items in the developer notes for human arbitration.

### Senior Reviewer Prompt

Use this exact prompt for the subagent (Locked to prevent drift):

```text
You are a highly experienced code reviewer specializing in Git patches and codebase architecture. Your task is to analyze the provided Git patch and provide comprehensive feedback. Focus on identifying potential bugs, architectural issues, inconsistencies, security vulnerabilities, and areas for improvement in code style and readability. Your response should be detailed and constructive, offering specific suggestions for remediation where applicable. Prioritize clarity and conciseness in your feedback.

# Step by Step Instructions

1. Read the provided patch carefully. Understand the changes it introduces to the codebase.
2. You have access to tools to read files and search the codebase. You SHOULD use these tools to read the full content of modified files and check the surrounding context or related files if the diff alone is insufficient to judge architecture or correctness.
3. Analyze the patch for potential issues:
    - Functionality: Does the code work as intended? Are there any bugs or unexpected behavior?
    - Architecture & Design: Does the code follow proper layering, component boundaries, and project patterns (e.g., pRPC queries, React Query usage)? Are there any circular dependencies or abstraction leaks?
    - Security: Are there any security vulnerabilities introduced by the patch?
    - Style & Maintainability: Does the code adhere to the project's coding style guidelines? Is it readable and maintainable? Avoid coupling tests to internal UI library classes (like .Mui...) if semantic roles or test IDs can be used.
    - Consistency: Are there any inconsistencies with existing code or design patterns?
    - Styling Drift & A11y: Does the patch add raw hex colors, `borderRadius`, `boxShadow`, or `fontSize` literals in components, copy an `sx` block that a theme variant or recipe in `src/fleet/theme/` already covers (`metricTileSx`, `subMetricRowSx`, `surfaceCardSx`), apply a `9999px` pill radius to a multi-line block, suppress keyboard focus rings (`outline: 'none'` on `:focus-visible`), omit `aria-label` on an icon-only `<IconButton>`, hand-roll a status chip instead of `<StatusPill>`, or nest a bordered card inside another? Point to the token, recipe, or variant to use (see `src/fleet/theme/README.md`).
    - Deprecation & Anti-Mimicry: Does the patch introduce new usages of JSDoc `@deprecated` symbols or `*_OLD` / `*_deprecated` files?
    - Data Dependency Alignment: If the patch modifies APIs to support UI features, does it fully satisfy the UI's data requirements without forcing redundant calls?
    - Frontend Review Invariants:
        * Cursor Semantics vs. Click Affordance: Use `cursor: 'help'` (or `'default'`) on tooltip-only triggers, read-only chips, and progress bars that have no click action. Reserve `cursor: 'pointer'` strictly for clickable controls.
        * Keyboard Accessibility (`a11y`) for Tooltips & Icon-Only Controls: Always add `tabIndex={0}` to non-button `<Tooltip>` trigger children (`<Box>`, `<Typography>`, `<span>`) so keyboard users can open tooltips. Always provide a descriptive `aria-label` on icon-only links (`<a>`) and buttons (`<IconButton>`).
        * Click Event Bubbling in Selectable Rows: Never call `e.stopPropagation()` on passive/unclickable child badges or chips inside clickable `<ListItem>` or table rows, as it swallows clicks and prevents row selection.
        * Direct Module Imports Over `.tsx` Re-Exports: Never re-export utility functions or formatting constants from `.tsx` React component files. Import utilities and constants directly from `src/fleet/utils/` or `src/fleet/constants/`.
        * `ts-proto` `int64` String Defaults: `ts-proto` deserializes `int64` fields as strings where the default/missing value is `'0'` (not `''`). Check `val && val !== '0'` and use `'0'` in mock fixtures.
        * No Redundant `pageSize: 1` Summary RPCs: Never fire a separate `pageSize: 1` `List*` RPC solely to read a total or summary count that is already returned by the primary table query on the same page.
        * Deterministic Test Mock Cleanup: Always restore mocks (`jest.restoreAllMocks()` / `mockRestore()`) inside `afterEach()` hooks rather than at the end of `it()` / `test()` bodies so assertion failures never leak mocks into subsequent tests.
    - Commit Message, Visual Proof & Screenshot Authenticity:
        * Top-of-Description Visual Block: For visual UI changes, verify `Demo: https://<version>-dot-luci-milo-dev.appspot.com/ui/fleet/p/...`, `UI Diff: http://go/ui-diff?before=<id>&after=<id>&title=<Title>`, and `Before:` / `After:` `http://screen/<id>` links appear at line 1 immediately below the title (before `Why:` and `Context:`, never buried at the bottom near `Bug:`).
        * Zero Fake Mock UIs or Crash Canvases: Reject standalone HTML mockup cards, stitched side-by-side images (`sbs.png`), temporary in-app `"Preview Harness"` components, DOM-edited mockups, or crashed React error screens uploaded as `Before:`/`After:` screenshots.
        * 15-Second `Why:` & `Context:` (`<= 100` words combined): Verify `Why:` (`<= 35` words, zero file paths or code jargon) and `Context:` (`<= 65` words, `<= 3` bullets using basename file references only), `Bug: b/<id>` slash syntax (never `Bug: b:<id>`), and zero duplicate concatenated commit messages.
    - Testing: Does the patch include sufficient tests to cover the changes? Are E2E tests using stable practices (mocking, waiting for intercepts)?
4. Formulate concise and constructive feedback for each identified issue. Provide specific suggestions for remediation where possible.
5. Summarize your findings. You MUST structure your output using the following schema:

## Review Summary
[Pass/Fail/Needs Revision]

### 🚨 Critical Issues (Bugs, Breaks, Security, Major Architecture)
- ...
### ⚠️ Maintenance & Consistency (Style, Drift, Minor Architecture)
- ...
### 💡 Minor Optimizations (Readability, Nits)
- ...

6. Review the feedback written so far. Is the feedback comprehensive and sufficiently detailed? If not, go back to step 2 to check for missing context or step 3 to re-evaluate.
7. Output the complete review.
```

### Example Tool Call

**Note**: When invoking the subagent via JSON tool call, ensure that any multi-line strings (like the prompt and the git diff) are properly escaped (e.g., replacing real newlines with `\n` and escaping quotes).

```json
{
  "toolSummary": "Invoke senior reviewer subagent",
  "toolAction": "Invoking subagent",
  "Subagents": [
    {
      "TypeName": "self",
      "Role": "Senior Code Reviewer",
      "Prompt": "<Insert Full Senior Reviewer Prompt from section above>\n\n[Insert git diff here]"
    }
  ]
}
```

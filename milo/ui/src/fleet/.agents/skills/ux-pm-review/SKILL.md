---
name: ux-pm-review
description: Conducts multi-perspective Product Management, UX Design, UXR, and QA reviews of new components, prototypes, or user flows across all Fleet Console features. Use before declaring a UX task complete or uploading a CL with visual/interaction changes.
---

# UX & Product Manager Review Skill

> **Note**: This document contains instructions for AI code assistants working in this repository. Human developers can use it as a reference.

Use this skill to conduct a comprehensive multi-perspective UX, Product Management, and QA audit of frontend changes across any Fleet Console page, component, or user flow before declaring a task complete or uploading a CL.

---

## Workflow

> [!IMPORTANT]
> **At the start of a UX & PM Review**, copy this progress checklist into your very next response to the user, and check off the steps sequentially:

Progress:
- [ ] Step 1: Prepare Code Diff & Prototype URL / Screenshots
- [ ] Step 2: Launch UX & PM Review Subagent with Evaluation Criteria
- [ ] Step 3: Process Review Findings & Fix Polish / Layout / Copy Issues (limit to 3 loops)
- [ ] Step 4: Re-verify against Laws of UX & Presubmit Suite ([project-verification](../project-verification/SKILL.md))

- **Exit Condition**: Limit the review-remediation cycle to a maximum of 3 iterations. If the reviewer still flags minor issues after 3 iterations, stop the loop, proceed with presubmits (`npm run lint`, `npm test`, `npm run type-check`), and document remaining open items in developer notes for human arbitration.

---

## Temp File & Artifact Hygiene

> [!NOTE]
> All transient subagent review reports, diff dumps, and intermediate QA feedback logs MUST be written strictly to `milo/ui/.tmp/ux-reviews/` (relative to repository root). Do not pollute workspace directories with scratch files.

---

## 1. Multi-Persona Evaluation Framework

When spawning a review subagent via `invoke_subagent`, evaluate the changes across 4 generalized core perspectives:

### 1. Principal Product Manager (PM)
- **Persona Alignment**: Does the feature serve key Fleet Console personas (Hardware Technicians, Lab Leads, Release Engineers, Resource Planners, and Fleet Admins)?
- **Core User Journey (CUJ) Completeness**: Are all relevant user journeys supported end-to-end with intuitive entry points, clear breadcrumb navigation, and zero dead ends?
- **Feature & Domain Boundary Integrity**: Does the design respect domain boundaries (e.g. keeping inventory devices, repair tasks, catalog specs, and resource requests cleanly separated while providing seamless deep-links)?

### 2. Staff UX & Visual Designer
- **Visual Token Compliance**: Adherence to native Fleet Console design tokens (elevation `0`/`1`, clean `1px solid ${theme.palette.divider}` borders, Google Sans / Roboto typography scale, cohesive HSL color palettes).
- **Card Nesting & Layout Hierarchy**: Follow layout spacing and container grouping rules in [writing-and-ux-principles](../writing-and-ux-principles/SKILL.md) and [high-density-ui](../high-density-ui/SKILL.md) (zero card-within-a-card heavy nesting, zero text overlap or button clipping).
- **Laws of UX & Gestalt Laws**: Compliance with [writing-and-ux-principles](../writing-and-ux-principles/SKILL.md) (Hick's Law decision time, Fitts's Law touch targets, Miller's Law / Cowan's limit metric chunking, Gestalt Proximity & Common Region).

### 3. UX Researcher (UXR)
- **Cognitive Load & Information Architecture**: Is information chunked logically into digestible visual sections? Are raw AIP-160 filter expressions or internal backend schema keys replaced with clear, human-readable UI labels?
- **Safety & Error Prevention**: Are confirmation guardrails provided for high-risk, destructive, or state-clearing actions (e.g. confirmation dialogs when discarding unsaved scope filters or initiating bulk device actions)?

### 4. QA Engineer
- **Edge-Case & Boundary Handling**:
  - **Authentication & Authorization**: Unauthenticated / Anonymous sessions (`isAnonymous: true`) gracefully hide protected internal resources, gate sensitive telemetry behind login prompts, and prevent crashing.
  - **URL & State Synchronization**: Search parameters, filter queries, modal states, and tab selections synchronize bidirectionally with the URL, supporting browser back/forward navigation and shareable links.
  - **Degraded & Empty States**: Network errors, zero search results, or degraded RPC responses render clean empty state components (e.g. permission warning banners, helpful empty table messages) without `NaN%`, `undefined`, or division-by-zero crashes.
  - **State Persistence**: Local storage preferences and session defaults sync reliably across tab switches and page refreshes.

---

## 2. Reviewer Subagent Prompt Template

*Note: Replace `<file_paths>` with the actual relative file paths under review before calling `invoke_subagent`.*

> [!IMPORTANT]
> **Subagent Skill Attachment**: When launching subagents via `invoke_subagent`, you MUST explicitly include the absolute paths of relevant skills in their prompt and command them to read those files using `view_file` with `IsSkillFile: true` (boolean primitive) before beginning their work.
>
> **Note**: When constructing JSON tool calls for `invoke_subagent`, ensure that multi-line strings in prompt arguments are properly escaped (e.g., replacing real newlines with `\n` and escaping double quotes).

#### Example Reviewer Invocation (JSON Schema)
```json
{
  "toolSummary": "Launch UX and PM review subagent",
  "toolAction": "Invoking review subagent",
  "Subagents": [
    {
      "TypeName": "self",
      "Role": "Staff UX & Senior PM Reviewer",
      "Prompt": "Before beginning your review task, you MUST view and adhere to the following skills by calling view_file with IsSkillFile set to the boolean primitive true:\n- /absolute/path/to/milo/ui/src/fleet/.agents/skills/writing-and-ux-principles/SKILL.md\n- /absolute/path/to/milo/ui/src/fleet/.agents/skills/high-density-ui/SKILL.md\n- /absolute/path/to/milo/ui/src/fleet/.agents/skills/ux-pm-review/SKILL.md\n\nPerform a comprehensive UX, Product Management, and QA review of the changes in:\n- <file_paths>\n\nEvaluate against:\n1. Product & Business Value: Value for target Fleet Console user personas and end-to-end CUJ completeness.\n2. Visual & Structural Polish: Compliance with Material Design 3 tokens, layout hierarchy, card nesting hygiene, and Laws of UX (Hick's, Fitts's, Miller's/Cowan's, Gestalt Proximity).\n3. Copy & Writing Standards: Compliance with Orwell's Writing Rules and Material Design Writing Spec (zero raw AIP-160/backend jargon in user-facing labels).\n4. QA Boundary Conditions: Anonymous sessions, URL state synchronization, empty/degraded data states, and layout responsiveness.\n\nSave intermediate review notes to milo/ui/.tmp/ux-reviews/review_report.md.\nReturn your formal Review Report with clear PASS / REQUIRES POLISH verdicts and exact diff recommendations."
    }
  ]
}
```

---

## Related Skills
- [design-tournament](../design-tournament/SKILL.md)
- [writing-and-ux-principles](../writing-and-ux-principles/SKILL.md)
- [ux-prototyping](../ux-prototyping/SKILL.md)
- [high-density-ui](../high-density-ui/SKILL.md)
- [project-verification](../project-verification/SKILL.md)

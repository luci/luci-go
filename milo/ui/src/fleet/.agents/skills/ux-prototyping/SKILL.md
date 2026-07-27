---
name: ux-prototyping
description: Guidelines for rapid prototyping and adhering to UX principles in Fleet Console. Use this skill when creating new UI views, modifying existing layouts, or performing rapid prototyping for UX changes across any Fleet Console page or component.
---

# UX Prototyping Skill

> **Note**: This document contains instructions for AI code assistants working in this repository. Human developers can use it as a reference.

Use this skill when creating new UI views, modifying existing layouts, or performing rapid prototyping for UX changes across any feature or page in the Fleet Console.

---

## Workflow

> [!IMPORTANT]
> **At the start of a UX Prototyping task**, copy this progress checklist into your very next response to the user, and check off the steps sequentially:

Progress:
- [ ] Step 1: Design Tournament (If open-ended / major UI feature)
- [ ] Step 2: Build UI Layout with Material 3 & Cognitive UX Principles
- [ ] Step 3: Run UX & PM Review Subagent Pass
- [ ] Step 4: Run Verification Suite & Presubmits

---

## Temp File & Artifact Hygiene

> [!NOTE]
> All transient prototype scratchpads, mock dataset dumps, and draft wireframe files MUST be written strictly to `milo/ui/.tmp/ux-prototyping/` (relative to repository root). Never commit unscrubbed prototyping data to public git branches.

---

## Core Principles

1. **Adhere to Cognitive UX & Writing Guidelines**: Follow [writing-and-ux-principles](../writing-and-ux-principles/SKILL.md) for Orwell's 6 writing rules, Material Design writing guidelines, and Laws of UX (Hick's, Fitts's, Miller's/Cowan's, Gestalt Proximity).
2. **Use Material-UI (MUI)**: Leverage MUI components (`Box`, `Grid2`, `Typography`, `Paper`, `Chip`, `Button`) with native Material Design 3 tokens.
3. **Card Nesting Hygiene**: Follow layout spacing and container grouping rules in [writing-and-ux-principles](../writing-and-ux-principles/SKILL.md) and [high-density-ui](../high-density-ui/SKILL.md) (zero card-within-a-card heavy nesting, zero text overlap or button clipping).
4. **Mock Data Sanitization**: Store temporary prototyping mock datasets in `milo/ui/.tmp/ux-prototyping/`. Never commit sensitive internal tokens or non-public data to open-source repository files.

---

## Detailed Workflow Steps

1. **Design Tournament (For Open-Ended / Complex UI Tasks)**:
   - For open-ended design problems or major page overhauls, host a multi-persona [design-tournament](../design-tournament/SKILL.md) between competing design subagents (e.g. M3 Specialist, High-Density Enterprise, Ergonomics & Accessibility) judged by a PM panel to synthesize the best visual approach.

2. **Rapid Prototyping & Layout Implementation**:
   - Build UI components using clean Material 3 surfaces.
   - Group information into digestible visual sections of 4 ± 1 metric scorecards (Cowan's working memory limit / Miller's Law chunking).
   - Ensure zero text overlap, button clipping, or negative margin bleed across container boundaries across all screen breakpoints.

3. **Mandatory Subagent Review Passes**:
   - Before finalizing any prototype or submitting a CL, run a structured [ux-pm-review](../ux-pm-review/SKILL.md) subagent pass to evaluate Product Manager alignment, UX visual polish, Orwell/Material writing rules, and QA boundary conditions (anonymous sessions, URL state synchronization, empty/degraded data states).

4. **Project Verification & Presubmits**:
   - Run the complete verification suite defined in [project-verification](../project-verification/SKILL.md) (`npm run lint`, `npm test`, `npm run type-check`).

---

## Related Skills
- [design-tournament](../design-tournament/SKILL.md)
- [writing-and-ux-principles](../writing-and-ux-principles/SKILL.md)
- [ux-pm-review](../ux-pm-review/SKILL.md)
- [high-density-ui](../high-density-ui/SKILL.md)
- [project-verification](../project-verification/SKILL.md)

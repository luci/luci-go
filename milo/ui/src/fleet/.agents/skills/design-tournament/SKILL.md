---
name: design-tournament
description: Organizes, executes, and judges multi-persona UX Design Tournaments to explore alternative UI architectures, visual treatments, and interaction models across Fleet Console features. Use when creating new complex UI views, overhauling existing pages, or resolving open-ended UX design decisions.
---

# UX Design Tournament Skill

> **Note**: This document contains instructions for AI code assistants working in this repository. Human developers can use it as a reference.

Use this skill whenever you are tasked with designing a major new UI view, overhauling an existing page or dashboard, or resolving open-ended visual/structural UX trade-offs anywhere in the Fleet Console.

A **UX Design Tournament** prevents single-agent design tunnel vision by generating competing design proposals from specialized subagent personas, evaluating them against objective UX principles, and synthesizing the winning elements into a single high-polish implementation.

---

## Workflow

> [!IMPORTANT]
> **At the start of a Design Tournament**, you MUST copy the progress checklist below into your very next response to the user, and check off the steps sequentially as you complete them.

Progress:
- [ ] Step 1: Define Tournament Criteria & Design Challenge
- [ ] Step 2: Launch Multi-Persona Design Competitors (M3, Enterprise Ops, Ergonomics)
- [ ] Step 3: Launch PM & UX Judging Panel Subagent
- [ ] Step 4: Synthesize Winning Blueprint & Validate against Laws of UX

---

## Temp File & Artifact Hygiene

> [!NOTE]
> All transient subagent design proposals, wireframe drafts, and intermediate evaluation matrix files MUST be written strictly to `milo/ui/.tmp/design-tournaments/` (relative to repository root). Do not pollute workspace directories with scratch files.

---

## Detailed Tournament Phases

### Phase 1: Define Criteria & Design Challenge
Establish clear, quantifiable criteria before generating designs:
1. **Material Design 3 & FCon Design Tokens**: Elevation (`0`/`1`), clean borders (`1px solid ${theme.palette.divider}`), Google Sans / Roboto typography scale, color harmony.
2. **Laws of UX Compliance**: [writing-and-ux-principles](../writing-and-ux-principles/SKILL.md) (Hick's Law decision time, Fitts's Law touch targets, Miller's Law / Cowan's limit metric chunking, Gestalt Proximity & Common Region).
3. **Writing & Tone**: Orwell's 6 Writing Rules, Material Design Writing Spec (zero raw AIP-160/backend micro-syntax in user labels).
4. **Visual Polish**: Follow layout spacing and container grouping rules in [writing-and-ux-principles](../writing-and-ux-principles/SKILL.md) and [high-density-ui](../high-density-ui/SKILL.md) (zero card-within-a-card heavy nesting, zero text overlap or button clipping).

### Phase 2: Launch Multi-Persona Design Competitors
Invoke 3-4 subagents concurrently using `invoke_subagent` with distinct design personas tailored to the design challenge:

```text
Persona Competitors Example:
1. Persona A: "Material Design 3 Specialist"
   - Focus: Native M3 surface hierarchy, subtle elevation tokens, fluid responsive grid layouts, and consistent spacing.
2. Persona B: "High-Density Enterprise Ops Specialist"
   - Focus: Maximum operational data density, 1-click quick-action triage bars, compact data tables, and zero visual fluff.
3. Persona C: "Ergonomics & Accessibility Specialist"
   - Focus: WCAG AA color contrast, low cognitive load, keyboard focus rings, accessible touch targets (≥32px), and clear status badges.
```

Each competitor subagent must output a complete high-fidelity design specification covering:
- Visual anatomy & layout wireframe
- Typography & color palette
- Component hierarchy & interaction model
- Rationale & design trade-offs

Save intermediate proposals to `milo/ui/.tmp/design-tournaments/proposal_<persona_name>.md`.

> [!IMPORTANT]
> **Subagent Skill Attachment**: When launching subagents via `invoke_subagent`, you MUST explicitly include the absolute paths of relevant skills in their prompt and command them to read those files using `view_file` with `IsSkillFile: true` (boolean primitive) before beginning their work.
>
> **Note**: When constructing JSON tool calls for `invoke_subagent`, ensure that multi-line strings in prompt arguments are properly escaped (e.g., replacing real newlines with `\n` and escaping double quotes).

#### Example Competitor Invocation (JSON Schema)
```json
{
  "toolSummary": "Launch M3 specialist design competitor",
  "toolAction": "Invoking design subagent",
  "Subagents": [
    {
      "TypeName": "self",
      "Role": "Material Design 3 Specialist",
      "Prompt": "Before beginning your design task, you MUST view and adhere to the following skills by calling view_file with IsSkillFile set to the boolean primitive true:\n- /absolute/path/to/milo/ui/src/fleet/.agents/skills/writing-and-ux-principles/SKILL.md\n- /absolute/path/to/milo/ui/src/fleet/.agents/skills/high-density-ui/SKILL.md\n\nGenerate a high-fidelity design specification for <feature_name> focusing on Material Design 3 surface hierarchy and elevation tokens. Save your proposal to milo/ui/.tmp/design-tournaments/proposal_m3.md."
    }
  ]
}
```

### Phase 3: Launch PM & UX Judging Panel Subagent
Invoke a judge subagent (`TypeName: "self"`, `Role: Staff UX & Senior PM Reviewer`) to evaluate all competitor proposals against the criteria established in Phase 1.

The judge subagent produces a comparative matrix saved to `milo/ui/.tmp/design-tournaments/evaluation_matrix.md`:
- Persona scoring across Density, Visual Polish, Accessibility, and Cognitive Load
- Pass/Fail evaluation against Laws of UX and Material Writing Specs
- Declaration of winning features from each proposal

#### Example Judge Invocation (JSON Schema)
```json
{
  "toolSummary": "Launch PM and UX judging panel",
  "toolAction": "Invoking judging subagent",
  "Subagents": [
    {
      "TypeName": "self",
      "Role": "Staff UX & Senior PM Reviewer",
      "Prompt": "Before beginning your evaluation task, you MUST view and adhere to the following skills by calling view_file with IsSkillFile set to the boolean primitive true:\n- /absolute/path/to/milo/ui/src/fleet/.agents/skills/writing-and-ux-principles/SKILL.md\n- /absolute/path/to/milo/ui/src/fleet/.agents/skills/high-density-ui/SKILL.md\n\nEvaluate all competitor proposals in milo/ui/.tmp/design-tournaments/ against Laws of UX and Material Design writing rules. Output an evaluation matrix to milo/ui/.tmp/design-tournaments/evaluation_matrix.md."
    }
  ]
}
```

### Phase 4: Synthesize Winning Blueprint
Synthesize the best aspects of all proposals into a single unified implementation plan:
- Take visual elegance and elevation tokens from the M3 proposal
- Take high-density triage and quick action bars from the Enterprise Ops proposal
- Take accessibility & touch target guardrails from the Ergonomics proposal
- Validate the synthesized plan against [project-verification](../project-verification/SKILL.md) before writing code.

---

## Related Skills
- [writing-and-ux-principles](../writing-and-ux-principles/SKILL.md)
- [ux-pm-review](../ux-pm-review/SKILL.md)
- [ux-prototyping](../ux-prototyping/SKILL.md)
- [high-density-ui](../high-density-ui/SKILL.md)

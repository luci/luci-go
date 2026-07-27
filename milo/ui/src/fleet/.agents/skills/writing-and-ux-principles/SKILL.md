---
name: writing-and-ux-principles
description: Standardizes UI copy writing guidelines (George Orwell's Writing Rules & Material Design Writing Spec) and cognitive UX principles (Laws of UX & Gestalt Laws) across all Fleet Console features. Use whenever authoring user-facing text, tooltips, status badges, dialogs, or establishing visual layout hierarchy.
---

# Writing & UX Principles Skill

> **Note**: This document contains instructions for AI code assistants working in this repository. Human developers can use it as a reference.

Use this skill whenever you are authoring user-facing UI text, crafting tooltips/notifications/modals, or structuring component hierarchy and layout spacing across any feature in the Fleet Console.

---

## Workflow

> [!IMPORTANT]
> **At the start of authoring UI text or component layouts**, copy this progress checklist into your very next response to the user, and check off the steps sequentially:

Progress:
- [ ] Step 1: Audit Copy against Orwell's 6 Writing Rules & Material Writing Spec
- [ ] Step 2: Audit UI Hierarchy against Laws of UX (Hick's, Fitts's, Miller's, Jakob's)
- [ ] Step 3: Audit Spacing & Grouping against Gestalt Laws of Proximity & Common Region
- [ ] Step 4: Verify Zero AIP-160 Backend Jargon in User-Facing Labels

---

## Temp File & Artifact Hygiene

> [!NOTE]
> All transient copy audit notes, draft text alternatives, and layout wireframe scratchpads MUST be written strictly to `milo/ui/.tmp/ux-writing/` (relative to repository root). Do not pollute workspace directories with scratch files.

---

## 1. George Orwell's 6 Rules for Software Interfaces

Apply Orwell's rules universally across all Fleet Console domain pages:

1. **Never use a metaphor, simile, or other figure of speech which you are used to seeing in print**:
   - Keep software messaging literal, objective, and clear across all catalog specs, inventory logs, and resource insights.
2. **Never use a long word where a short one will do**:
   - Bad: "Utilize the filtering functionality below"
   - Good: "Filter devices"
   - Bad: "Execute resource allocation request"
   - Good: "Request capacity"
3. **If it is possible to cut a word out, always cut it out**:
   - Bad: "Preset filter scoping is currently active. Select another option below to switch:"
   - Good: "Select an option to switch scope:"
   - Bad: "Displaying total count of 24,180 total devices"
   - Good: "24,180 devices"
4. **Never use the passive where you can use the active**:
   - Bad: "15 devices are required to be triaged by technicians"
   - Good: "Triage 15 offline devices →"
   - Bad: "Resource request was approved by admin"
   - Good: "Admin approved request"
5. **Never use a foreign phrase, a scientific word, or a jargon word if you can think of an everyday English equivalent**:
   - Bad: `fc_is_offline = true` (Repairs) / `device_model_name` (Catalog) / `requested_qps_quota` (Requests) / `widget_refresh_ms` (Dashboards)
   - Good: "Offline devices" / "Model name" / "Requested quota" / "Refresh interval"
6. **Break any of these rules sooner than say anything outright barbarous**:
   - Avoid alarmist security/error warnings for simple software scoping concepts or non-destructive user actions.

---

## 2. Material Design Writing & Communication Spec

Follow the official Google Material Design Writing Guide:
- **User-Centric & Objective-First**: Lead with the action or benefit to the user.
  - *Catalog*: "Browse product catalog (48) →"
  - *Repairs*: "Triage offline devices (15) →"
  - *Requests*: "Create resource request →"
- **Direct User Address**: Use "you" and "your" when talking to the user.
  - *Example*: "Save as your default view" / "Your active resource requests"
- **Clean Numeral & Percentage Formatting**:
  - Format large numbers with locale separators (`29,087 devices`, `70,893 total`).
  - Format percentages cleanly with 1 decimal place (`94.2%`), avoiding raw float representation (`94.238472%`).
- **Sentence Case**: Use sentence case for UI labels, section titles, table column headers, and action buttons (except proper nouns).
  - *Example*: "Total fleet capacity" (Sentence case) vs "TOTAL FLEET CAPACITY" (Uppercase reserved for small overline category tags only).

---

## 3. Laws of UX & Gestalt Layout Principles

Adhere strictly to cognitive ergonomics guidelines across all Fleet Console pages:

### A. Laws of UX
- **Hick's Law (Decision Time)**: Minimize the number of choices presented at once to reduce cognitive overload.
  - *Implementation*: In filter dropdowns or menu selectors, display a clean 2-line layout (Title + Badge + Count + Subtitle) instead of a chaotic cluster of competing pills and action buttons.
- **Fitts's Law (Target Accessibility)**: Interactive touch/click targets must be sufficiently large (minimum `32px` height) and positioned near natural cursor movement paths.
- **Miller's Law (7 ± 2) & Cowan's Working Memory Limit (4 ± 1)**: While Miller's Law famously identifies 7 ± 2 chunks as the limit of short-term memory, Nelson Cowan's research demonstrates that working memory capacity for visual chunks is closer to **4 ± 1**. Group operational metrics into digestible visual chunks of 4 ± 1 scorecards or summary cards per section to prevent cognitive overload.
- **Jakob's Law (Mental Models)**: Users expect Fleet Console to behave like standard modern web applications.
  - *Implementation*: Use standard search bar scope prefix segments and standard table layouts rather than custom un-intuitive UI widgets.

### B. Gestalt Layout Principles
- **Law of Proximity**: Place related controls and text close together. Keep titles, status badges, and action buttons on a unified horizontal header row.
- **Law of Common Region**: Group related content inside clean, light-bordered containers (`border: '1px solid ${theme.palette.divider}'`, `backgroundColor: theme.palette.background.paper`). Avoid nesting cards inside cards ("card-within-a-card" look) which creates heavy visual noise. For page-level or standalone surface containers, use generous whitespace (`p: 2.5` to `p: 3`), while operational data tables, triage bars, and metrics grids must follow [high-density-ui](../high-density-ui/SKILL.md) compact padding (`2px 4px`).

---

## Related Skills
- [design-tournament](../design-tournament/SKILL.md)
- [ux-pm-review](../ux-pm-review/SKILL.md)
- [ux-prototyping](../ux-prototyping/SKILL.md)
- [high-density-ui](../high-density-ui/SKILL.md)

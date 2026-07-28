# UX Architecture & Cognitive Load Principles

## Context
Fleet operators manage complex lab infrastructure under operational pressure. Interfaces must reduce triage fatigue rather than flood operators with raw data.

This document defines user experience rules for Fleet Console layouts and agent-generated designs.

## Principles

We evaluate all UI designs, CLs, and agent layouts using **Information-to-Decision Routing**.

### 1. Information-to-Decision Ratio (IDR)
When deciding which columns, filters, or cards to show, engineers must minimize the Information-to-Decision Ratio:

```
IDR = (Visual Data Displayed) / (Relevance to the Immediate Decision)
```

Target an IDR of **1:1** for any operational task.

#### OODA Loop Triage Model:
1. **Trigger**: What event or alert brings the operator to this screen?
2. **Minimum Viable Data (MVD)**: What minimal data points must the operator compare to understand the issue? (e.g., current state vs. expected state).
3. **Action**: What downstream action (Reboot, Deprovision, File Bug) must the operator take?
4. **Closing the Loop**: Can the operator execute the action directly on this screen?

**Rule**: If data does not help the user orient or decide on an immediate action, move it to secondary views (detail drawers, tooltips, or expandable panels).

---

### 2. High-Density Enterprise Layouts
Fleet Console targets professional operators who require high data throughput:
- **Compact Components**: Use compact table rows, text fields, and buttons (reducing padding by 30–50%).
- **Data-to-Ink Ratio**: Devote screen area to live status data, hostnames, and lab metrics rather than decorative borders or whitespace.
- **Grid Discipline**: Use an 8px grid system, dropping to 4px for tight component spacing.

---

### 3. Gestalt Grouping Principles
Dense layouts must be scannable at a glance:
- **Proximity**: Group related metadata (asset IDs and host statuses) tightly. Place action buttons directly next to the targets they affect.
- **Similarity**: Standardize component appearance. Elements with identical visual styling must behave identically.
- **Common Region**: Use subtle card backgrounds or alternating row colors to visually separate distinct data categories (e.g., Host Telemetry vs. Lab Controls).
- **Continuity**: Align text blocks and table columns strictly. Misalignment slows down scanning patterns.

---

### 4. Smart Defaults over Configuration Panels
Avoid adding configuration panels to resolve layout disagreements.
- **Opinionated Defaults**: Use telemetry and operator feedback to design a default view optimized for 80% of daily workflows.
- **Layered Customization**:
  - *Level 1 (Default)*: Curated, opinionated primary view.
  - *Level 2 (Refinement)*: Persisted quick-filters and column toggles in `localStorage`.
  - *Level 3 (Deep Dive)*: Progressive disclosure (drawers, modals) holding complete system state for edge-case debugging.

---

### 5. Working Memory Limits (Miller's Law)
Operators can hold 7 ± 2 concepts in working memory.
- Limit primary viewports to 5–7 visual groups.
- Hide secondary data inside collapsible drawers or slide-out panels until contextually needed.

---

### 6. Fault-Tolerant UX
- **Mistake-Proofing (Poka-Yoke)**: Disable action buttons when required fields are missing. Prevent command execution that will fail downstream.
- **Clear Error Messages**: Explain what failed and how to fix it (e.g., *"This project name is already taken. Try another name"* instead of *"Error 409: Conflict"*).

---

### 7. AI Co-Design Guidelines
AI agents must act as proactive UX reviewers:
1. When asked to create a new layout or table, the agent **MUST NOT** immediately output code. It MUST analyze the request against the IDR model first.
2. Ask the user to clarify:
   - What event triggers this view and what action follows?
   - What is the minimum data required for the choice?
   - What single default layout covers 80% of uses?

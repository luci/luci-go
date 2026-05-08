# Principles: UX Architecture, Enterprise Density, and Cognitive Load Minimization

## Context
As engineering velocity increases, the primary bottleneck for feature delivery is product and UX decision-making. Unchecked UI growth introduces codebase complexity, visual noise, and fragmented user journeys.

Our primary users (Fleet Operators and Lab Administrators) manage highly complex, data-heavy systems under operational pressure. They do not suffer from a lack of data; they suffer from **information triage fatigue**.

This document establishes **User Experience (UX) as a deterministic system design constraint**. The human brain has fixed hardware limitations—specifically working memory capacity, visual scanning latency, and decision fatigue. This document provides a first-principles analytical framework to help engineers and AI agents evaluate, design, and defend interface decisions systematically during code reviews and design phases.

---

## Principles

We evaluate all user interface designs, **Changelists (CLs)**, and agent-generated layouts against a unified scientific model: **Information-to-Decision Routing**.

### 1. The Core Analytical Framework: The Information-to-Decision Ratio (IDR)

When deciding what data to show, where to show it, or how to configure a view (e.g., "which columns go on this table by default," "what goes on this dashboard," or "how to organize this form"), engineers must analyze the interface using the **Information-to-Decision Ratio**:

```text
IDR = (Volume of Visual Data Displayed) / (Relevance to the Immediate Operational Decision)
```

An optimal interface strives to make this ratio as close to **1:1** as possible for any given moment in a user's workflow.

To determine what belongs in the primary viewport from first principles, engineers must trace the user's path through the **OODA Loop (Observe, Orient, Decide, Act)**:

              [ COGNITIVE PROCESSING ]
[ OBSERVE ]               |              [ ORIENT ]
Raw Data Visible -------- | -----------> Mental Model of State
|                     |
|                     v
[ ACT ]                 |              [ DECIDE ]
Physical Execution <----- | ------------ Choice of Action
#### First-Principles Questions to Ask:
1.  **What is the trigger?** What exact event or anomaly forces the user to look at this screen?
2.  **What is the minimum viable data (MVD) required to Orient?** To understand *what* is wrong, what parameters must the user compare? (e.g., If a machine is failing, they need its current state vs. its expected baseline state. They do not need its serial number or asset tag at this step).
3.  **What is the downstream action?** What physical action (e.g., "Reboot", "Deprovision", "File Bug") is the user trying to take?
4.  **How do we close the loop?** Does the screen let them execute that action immediately, or does it force them to navigate elsewhere, breaking their mental state?

> **The Selection Rule:** If a piece of data does not directly help the user *Orient* or *Decide* on the immediate downstream action, it must be omitted from the primary view and relegated to secondary disclosure (e.g., a detail drawer, hover tooltip, or expandable panel).

---

### 2. High-Density Enterprise Design (Material Design 3 High-Density)
Our UI is designed for professional operators who require high data throughput.

*   **Material Design 3 (M3) Density Guidelines:** All UI implementations must conform to standard high-density layout guidelines. For LLM/agent generation, follow standard compact specifications for component padding and spacing.
*   **Enterprise Density Principles:** Where standard consumer interfaces use generous white space, we adapt for **high density**:
    *   **Compact Component Scaling:** Use compact variants of table rows, text fields, and buttons (e.g., reducing padding and margins by 30–50% to maximize screen real estate).
    *   **Data-to-Ink Ratio:** Maximize the "ink" devoted to actual data (status states, hostnames, lab metrics) while minimizing decorative UI borders or excessive whitespace.
    *   **Grid Discipline:** Maintain a rigid 8px grid system, dropping to a 4px grid strictly for tight, high-density component spacing.

---

### 3. Gestalt Principles for High-Density UI Layouts
To make dense layouts instantly scannable, we use Gestalt principles to group information logically:

[ ID: 92830 ]  [ Host: node-compute-1 ]  <-- Proximity: Related metadata grouped tightly
[ ID: 92831 ]  [ Host: node-compute-2 ]======================================= <-- Common Region: Thin border separating devices
[ Action: Repair ]  [ Action: Power ]    <-- Similarity: Action buttons share visual style
*   **The Principle of Proximity (Visual Association):**
    *   *Rule:* Keep related fields, like asset IDs and host statuses, tightly grouped. If an action button (e.g., "Repair" or "Power Cycle") affects a specific device card, place it directly inside or immediately adjacent to that card.
*   **The Principle of Similarity (Consistent Component Archetypes):**
    *   *Rule:* Elements that look similar must behave similarly. If primary actions are styled as standard filled buttons, do not use that style for neutral navigation links.
*   **The Principle of Common Region (Explicit Scoping):**
    *   *Rule:* In highly dense layouts, use subtle background cards, thin borders, or alternating row colors to "scope" distinct sets of information (e.g., separating "Host Telemetry" from "Lab Environmental Controls") to prevent visual bleeding.
*   **The Principle of Continuity (Expected Flow of Alignment):**
    *   *Rule:* Keep grids strictly aligned. Misaligned inputs or text blocks break reading scanning patterns, introducing visual noise and slowing down operators.

---

### 4. Sane Defaults over "Lazy Configurability"
When engineers disagree on what to show, the temptation is to build a configuration panel and let the user figure it out. This is often a failure of product definition.

*   **Configurability is a Trade-Off:** Every customization option we expose creates a split-path in our testing matrix, increases code complexity, and forces the user to act as their own UX designer.
*   **The Principle of Sane Defaults:** We must use our telemetry, community feedback loops, and direct communication with operators to define a default view optimized for the primary 80% operational workflow.
*   **Layered Customization:** We allow configuration only as an escape hatch, not as a replacement for good design.
    *   *Tier 1 (The Default):* Highly curated, opinionated, and scannable interface.
    *   *Tier 2 (The Refinement):* Quick-filters or column toggles that persist locally (e.g., in LocalStorage) so individual sub-teams can adapt the space to their distinct day-to-day focuses.
    *   *Tier 3 (The Deep Dive):* Progressive disclosure (drawers, modal inspector views) containing the complete unadulterated system state for edge-case debugging.

---

### 5. Working Memory Limits (Miller’s Law)
*   **The Constraint:** The human brain can only hold 7 ± 2 items in working memory.
*   **UI Implementation:** No high-density dashboard or command center should force an operator to track more than 5-7 moving concepts or primary visual groups at once. Use progressive disclosure (collapsible sections, hover details, slide-out panels) to hide secondary data until it is contextually relevant.

---

### 6. Error State Engineering (Fault-Tolerant UX)
*   **Poka-Yoke (Mistake-Proofing):** Disable the "Submit" or "Execute" buttons if required form fields are invalid or incomplete. Do not allow users to execute commands that will predictably fail downstream.
*   **Graceful Recovery:** If an error occurs, the UI must explain *what happened* in human-understandable terms and *how to fix it* (e.g., instead of `Error 409: Conflict`, use `This project name is already taken. Please try another name`).

---

### 7. Protocol for AI Co-Design and Interactive Questioning
AI agents are not passive "code-writing utilities." They are expected to act as proactive product-engineering co-designers.

#### Agent Behavior Mandate:
When a user asks an agent to design a new UI view, add columns to a table, build a form, or create a layout, **the agent MUST NOT immediately output code**. Instead, the agent is programmatically required to initiate a short co-design dialogue by analyzing the request against the IDR and Cognitive Processing models.

#### The Agent "Pre-Flight" Questionnaire:
Before writing components or code structures, the agent must ask the operator to clarify the following three points:

1.  **The Trigger & Downstream Action:** *"What event or alarm brings the operator to this view, and what exact physical action are they trying to execute as a result of viewing this data?"*
2.  **The Minimal Viable Information (MVD):** *"To make that choice, what are the absolute minimum data points they must compare? Can we hide the secondary metadata in a detail drawer to protect working memory?"*
3.  **The Default Rule:** *"Instead of adding another configuration setting or filter checkbox, what is the single logical default state we can implement that satisfies the 80% case?"*

#### Conversation Flow Example:
> **User:** "Write a React component for a new node telemetry table showing Status, Temp, FanSpeed, IP, Hostname, AssetTag, RackLocation, CPUUsage, MemoryUsage, and Uptime."
>
> **Agent:** *"Before I generate the layout, let's look at the Information-to-Decision Ratio (IDR) to make sure this stays scannable.
>
> When a user opens this telemetry table, what critical decision are they trying to make? If they are looking for overheating hardware, we should prioritize `Hostname`, `Status`, `Temp`, and `FanSpeed`, and hide static metadata like `AssetTag`, `IP`, and `RackLocation` inside an expandable details drawer to minimize extraneous cognitive load.
>
> What is the target operational decision? Let's refine the default layout together before I write the code."*

---

## Consequences

### Positive (What we gain):
*   **Objective Code Reviews:** UI discussions in CLs can shift away from subjective tastes to analytical, verifiable arguments.
*   **Proactive Agent Gatekeeping:** AI agents act as an automated first-line review, stopping poor UX layouts and redundant configurations before they reach human reviewers.
*   **Reduced UI Complexity:** Our codebase will feature fewer conditional rendering paths and fewer configuration state checks.
*   **Higher Operator Efficiency:** Fleet and Lab operators can triage and manage infrastructure faster with fewer operational errors.

### Negative/Neutral (What we must accept):
*   **More Rigorous Discovery:** We must spend more upfront time understanding user journeys to accurately determine what our "opinions" and smart defaults should be.
*   **Interactive Development Friction:** Operators must expect to spend a few turn cycles debating and aligning with AI agents on layout philosophy rather than receiving instant, copy-pasteable code blocks on the first prompt.

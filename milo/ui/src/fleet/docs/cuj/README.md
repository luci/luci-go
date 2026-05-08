# Guidance: How to Write Critical User Journeys (CUJs)

This directory contains Critical User Journeys (CUJs) for the Fleet Console. CUJs are a key UX technique used to ensure we are building the right features for our users.

---

## What is a Critical User Journey?

A CUJ captures **who** the user is, their critical **goal**, and the journey of **tasks** the user undertakes to achieve that goal.

Unlike broad User Experience Maps that cover emotional highs and full end-to-end lifecycles, CUJs zero in on **one high-impact task** or flow that defines whether a user succeeds or fails in their objective.

If a CUJ breaks, the product breaks for the user.

---

## Components of a CUJ

Every CUJ must include three parts:

### 1. The User
A specific persona or role (e.g., Lab Technician, Fleet Engineer). Grounding the journey in a concrete profile ensures we optimize for real-world human needs.

### 2. The Goal
A deeper need in the mind of the user.
*   **Answers WHY:** The deeper reason a user uses our product, not *how* they use it.
*   **Human:** Written in the voice of the user ("I want to...").
*   **Product Agnostic:** Focuses on the problem to be solved, which could be solved in different ways as technology evolves.

*Example:* "I want to get broken devices back into the test pool as fast as possible, so that developers can run their tests without delay."

### 3. Tasks
Specific steps toward completing the goal.
*   **Answers WHAT:** They explain what a user is doing.
*   **Feature Agnostic:** There should be room to re-imagine how a user completes a task.
*   **Span Present and Future:** Tasks can be both achievable and not achievable in the current product.

---

## Types of CUJs in Fleet Console

When prioritizing which journeys to document, consider these types of impact:

1.  **High-Traffic (Common Workflows):** Journeys that a large number of users go through regularly (e.g., searching for a device, checking status). Small friction here leads to massive toil at scale.
2.  **High-Impact / Mission-Critical:** Journeys that directly prevent capacity drops or release slips (e.g., urgent repairs, capacity planning).
3.  **OEC-Based (Metric-Critical):** Journeys that directly influence our primary success metrics (Overall Evaluation Criterion), such as reducing manual repair time or increasing asset utilization.

---

## Best Practices

*   **Stick to One Job-to-be-Done:** Zero in on one clear user goal. Trying to map multiple jobs in one journey dilutes focus.
*   **Highlight Only What's Critical:** Don't fall into the trap of mapping every click or edge case. Focus on the 4–6 touchpoints that truly influence the outcome.
*   **Describe Current vs. Future:** Document how the task is completed today (even if it involves legacy tools or manual steps) and highlight friction points we want to avoid in the future.
*   **Co-create:** Journey mapping works best when engineers collaborate with each other and draw on direct feedback from operators and technicians to capture the full reality.

---
name: continuous-improvement
description: Analyzes session friction, reviews logs, and drafts process/documentation improvements. Use at the end of a task, after completing a CL, or when encountering significant workflow friction.
---

# Continuous Improvement Skill

> **Note**: This document contains instructions for AI code assistants working in this repository. Human developers can use it as a reference.

Use this skill at the end of a session or after encountering significant friction to analyze bottlenecks and propose improvements to documentation and processes.

## Workflow

> [!IMPORTANT]
> **At the start of reflection and continuous improvement**, you MUST copy the progress checklist below into your very next response to the user, and check off the steps sequentially as you complete them.

Progress:
- [ ] Step 1: Reflect on the Session (transcript, logs, bottlenecks)
- [ ] Step 2: Identify process or documentation gaps
- [ ] Step 3: Draft Action Plan (documentation updates, script wrappers, non-interactive solutions)
- [ ] Step 4: Implement and stage improvements cleanly

## Detailed Procedures

1. **Reflect on the Session**:
   - Review the session transcript or logs.
   - Identify areas where you got stuck, encountered unexpected errors, or spent time discovering implicit knowledge.
2. **Identify Bottlenecks**:
   - Were there missing or outdated docs?
   - Did tools fail or require interactive input that blocked background tasks?
   - Were file paths or component relationships unclear?
3. **Draft Action Plan**:
   - Propose specific documentation updates (e.g., adding READMEs, updating guides).
   - Suggest process improvements (e.g., non-interactive flags, better script wrappers).
4. **Implement Improvements**:
   - Create a new CL with the proposed changes.
   - When proposing or updating agentic skills, follow the best practices in [authoring-skills](../authoring-skills/SKILL.md).
   - Avoid mixing process changes with feature work in the same CL if possible, or group them logically.


## Best Practices for Reflection

- **Be Constructive**: Focus on actionable improvements rather than just complaining about issues.
- **Consider Accessibility**: Ensure skills and docs are easily discoverable by future agents (e.g., by placing them in standard locations or listing them in a central README).
- **Automate Defaults**: Where possible, suggest solutions that avoid interactive prompts for background tasks using targeted explicit flags (e.g., `-f`, `--force`) or non-interactive environment overrides. Never recommend piping standard `yes` inputs blindly.

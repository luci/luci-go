# Fleet Console Agent Skills

This directory contains skills (instructions and guidelines) for AI agents working on the Fleet Console frontend.

## Available Skills

- [prepare-cl](./skills/prepare-cl/SKILL.md): Verifies, commits, and uploads code changes as a Gerrit CL. Use when you have completed a task, fixed a bug, or are ready to submit changes for review.
- [senior-reviewer](./skills/senior-reviewer/SKILL.md): Performs a self-review of changed code using a Senior Reviewer subagent. Use before uploading a CL or completing a feature to identify bugs, security issues, or style inconsistencies.
- [manual-testing](./skills/manual-testing/SKILL.md): Guidelines for manual testing the Fleet Console using the browser subagent. Use when you need to verify UI changes, filter interactions, or page loads in the actual browser.
- [aip160-filtering](./skills/aip160-filtering/SKILL.md): Guidelines for generating AIP-160 filter strings for Fleet Console. Use when you need to generate filter strings for RPC queries or update the URL with filters.
- [high-density-ui](./skills/high-density-ui/SKILL.md): Guidelines for building high-density enterprise UIs in Fleet Console. Use when designing or modifying complex dashboards and tables in Fleet Console that require high data density.
- [project-verification](./skills/project-verification/SKILL.md): Runs project checks including linter, tests, and type-checks to ensure no regressions. Use before committing changes, before uploading a CL, or when validating code correctness.
- [ux-prototyping](./skills/ux-prototyping/SKILL.md): Guidelines for rapid prototyping and adhering to UX principles in Fleet Console. Use when you need to create new UI views, modify existing layouts, or perform rapid prototyping for UX changes.
- [continuous-improvement](./skills/continuous-improvement/SKILL.md): Analyzes session friction, reviews logs, and drafts process/documentation improvements. Use at the end of a task, after completing a CL, or when encountering significant workflow friction.

## Usage

Agents should read the `SKILL.md` file for the relevant skill before performing tasks associated with that skill.

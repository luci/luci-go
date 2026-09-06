---
name: skill-reviewer
description: Evaluates, audits, and validates agent skills for Fleet Console against official best practices for portability, discoverability, scope isolation, duplication, writing quality, and empirical trial verification.
---

# Fleet Console Skill Quality Audit & Review Protocol

This skill provides a standardized checklist and review protocol for evaluating agent skills across Fleet Console frontend (`go/src/go.chromium.org/luci/milo/ui/src/fleet`) and backend (`go/src/infra/fleetconsole`) packages. It sits alongside `senior-reviewer` to ensure that agent skills maintain high writing quality, actionable mental models, and empirical verification.

---

## 1. Mandatory Fleet Console Quality Gates

Every agent skill MUST pass the following 11 quality gates before landing in the repository:

### Gate 1: Portability Audit (NO Hardcoded Local Paths)
- [ ] **No Absolute User Paths**: The skill MUST NOT contain hardcoded user paths (e.g. `/Users/username/...` or `/home/user/...`).
- [ ] **Relative & Environment Paths**: File paths must be relative to the repository root (e.g., `./src/fleet` or `./go/src/infra/fleetconsole`) or use standard environment variables (`$GOPATH`, `$INFRA_DIR`, `$PROJECT_ID`).

### Gate 2: Generalization Audit (Preventing Overfitting)
- [ ] **No Single-Incident Overfitting**: Guidance must NOT be overfitted to a specific bug ID (e.g., `b/123456`) or one-off incident endpoint.
- [ ] **Generic Placeholders**: Commands and code snippets MUST use clear, uppercase placeholders (e.g. `<PROJECT_ID>`, `<SERVICE_NAME>`, `<ZONE>`, `<TARGET_HOST_FQDN>`).

### Gate 3: Frontmatter & Discoverability Audit
- [ ] **Valid Frontmatter**: Must include YAML frontmatter with `name` and `description`.
- [ ] **Kebab-Case Naming**: The `name` field must be concise kebab-case matching the directory name (e.g., `name: skill-reviewer` in `.agents/skills/skill-reviewer/SKILL.md`).
- [ ] **Actionable Description**: The `description` MUST clearly specify **WHEN** and **WHY** an AI assistant should activate this skill.

### Gate 4: Operational Safety & Verification Audit
- [ ] **Destructive Command Guard**: If the skill contains destructive commands (`rm`, `gcloud delete`, `git reset`), it MUST include explicit safety warnings or require explicit user confirmation.
- [ ] **Falsification & Error Recovery**: The skill MUST instruct agents when to stop, pivot, or report failure if an assumption or command fails.

### Gate 5: Executable Quality Audit
- [ ] **Valid Syntax**: All shell, Python, TypeScript, and Go code blocks must be syntactically valid.
- [ ] **Tested Dependencies**: Code snippets should rely on established workspace dependencies rather than unverified third-party libraries.

### Gate 6: Empirical Command Runnable Test (Mandatory Live Execution)
- [ ] **Terminal Execution Verification**: EVERY terminal command, `gcloud` query, and script snippet in the skill MUST be actually executed in the terminal by the author/reviewer before landing.
- [ ] **Zero Flag/Syntax Failures**: Verify that commands do not fail due to missing flags, path typos, or broken environment variables.

### Gate 7: Empirical Subagent Trial & Correctness Benchmark
- [ ] **Pre-Defined Correctness Benchmark**: Define an explicit benchmark of **"What using the skill correctly looks like"** (Expected Trajectory, Falsification Criteria, Decision Matrix Outcomes) BEFORE launching a test.
- [ ] **Clean Room Trial Execution**: Launch a clean subagent given ONLY the skill and a test problem statement.
- [ ] **Benchmark Evaluation**: Compare the subagent's actual output against the predefined correctness benchmark to confirm the skill reliably produces the expected diagnostic result without hallucination.

### Gate 8: Skill Duplication Audit
- [ ] **Non-Duplicative Verification**: Verify that the proposed skill does NOT duplicate existing skills in `.agents/skills/`.
- [ ] **Consolidation**: If overlap exists, update and extend the pre-existing skill rather than creating a duplicate file.

### Gate 9: Fleet Console Team Scoping Rule
- [ ] **Local FCon Scoping by Default**: Every new skill MUST start off scoped locally to the Fleet Console domain (`go/src/go.chromium.org/luci/milo/ui/src/fleet`, `go/src/infra/fleetconsole`) by default.
- [ ] **Controlled External Scope**: Do NOT advertise or place skills in root global spaces until the skill has been validated and battle-tested within Fleet Console team workflows.

### Gate 10: Technical Writing & Prose Quality Audit
- [ ] **Direct Imperative Voice**: Uses clear, concise, active voice ("Run `gcloud...`", "Inspect `statusDetails`").
- [ ] **Zero Ambiguity & Fluff**: Eliminates vague phrasing, typos, or unnecessary conversational filler.
- [ ] **Markdown Formatting**: Well-structured GitHub Flavored Markdown with clean code fencing, headers, and comparison tables.

### Gate 11: Guidance & Mental Model Quality Audit
- [ ] **Clear Decision Architecture**: Includes a clear decision tree, flowchart, or state diagram illustrating the execution flow.
- [ ] **Underlying Rationale**: Explains **WHY** specific steps are taken (e.g., explaining why edge latency < 50ms signals a load balancer drop).
- [ ] **Fallback & Failure Protocols**: Provides explicit instructions for handling unexpected command outputs or edge cases.

---

## 2. Review Protocol & Evaluation Output Template

When reviewing a skill, structure your evaluation response using this template:

```markdown
# Fleet Console Agent Skill Review Report

## Summary
- **Target Skill**: `.agents/skills/<skill-name>/SKILL.md`
- **Overall Status**: [PASS / NEEDS REVISION]

## Detailed Quality Audit

1. **Portability**: [PASS / FAIL] (Details on path usage)
2. **Generalization**: [PASS / FAIL] (Details on placeholder usage vs overfitting)
3. **Discoverability**: [PASS / FAIL] (Details on YAML frontmatter & description)
4. **Operational Safety**: [PASS / FAIL] (Details on destructive commands or falsification gates)
5. **Executable Quality**: [PASS / FAIL] (Details on syntax & runnable examples)
6. **Command Runnable Test**: [PASS / FAIL] (Details on empirical shell execution)
7. **Subagent Benchmark Trial**: [PASS / FAIL] (Details on clean-room subagent trial vs expected correctness benchmark)
8. **Duplication Audit**: [PASS / FAIL] (Details on non-duplication vs existing skills)
9. **FCon Scoping**: [PASS / FAIL] (Details on local team scoping by default)
10. **Writing Quality**: [PASS / FAIL] (Details on imperative voice, clarity, and formatting)
11. **Guidance Quality**: [PASS / FAIL] (Details on decision trees, rationale, and fallback protocols)

## Actionable Recommendations
- Item 1: ...
- Item 2: ...
```

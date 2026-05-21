---
name: senior_reviewer
description: Instructions for running the SeniorReviewer subagent.
---

# Senior Reviewer Skill

> **Note**: This document contains instructions for AI code assistants working in this repository. Human developers can use it as a reference.

Use this skill to perform a self-review of your changes using a subagent.

## Instructions

To perform the mandatory self-review, follow these steps to invoke a subagent:

1. **Invoke Subagent**: Use the `invoke_subagent` tool with `TypeName: "self"` and the prompt below.
2. **Pass Diff**: It is recommended to include the output of `git diff` directly inside the prompt along with the review instructions to ensure the subagent has the full context immediately. This establishes a clear, single-prompt process.
3. **Address Feedback**: Iterate on the code until the reviewer subagent provides a clean review.
   - **Exit Condition**: Limit the cycle to a maximum of 3 iterations. If the reviewer still finds issues after 3 iterations, stop the loop, upload the CL, and flag the remaining items in the developer notes for human arbitration.

### Senior Reviewer Prompt

Use this exact prompt for the subagent (Locked to prevent drift):

```text
You are a highly experienced code reviewer specializing in Git patches. Your task is to analyze the provided Git patch and provide comprehensive feedback. Focus on identifying potential bugs, inconsistencies, security vulnerabilities, and areas for improvement in code style and readability. Your response should be detailed and constructive, offering specific suggestions for remediation where applicable. Prioritize clarity and conciseness in your feedback.

# Step by Step Instructions

1. Read the provided patch carefully. Understand the changes it introduces to the codebase.
2. Analyze the patch for potential issues:
    - Functionality: Does the code work as intended? Are there any bugs or unexpected behavior?
    - Security: Are there any security vulnerabilities introduced by the patch?
    - Style: Does the code adhere to the project's coding style guidelines? Is it readable and maintainable?
    - Consistency: Are there any inconsistencies with existing code or design patterns?
    - Testing: Does the patch include sufficient tests to cover the changes?
3. Formulate concise and constructive feedback for each identified issue. Provide specific suggestions for remediation where possible.
4. Summarize your findings. You MUST structure your output using the following schema:

## Review Summary
[Pass/Fail/Needs Revision]

### 🚨 Critical Issues (Bugs, Breaks, Security)
- ...
### ⚠️ Maintenance & Consistency (Style, Drift)
- ...
### 💡 Minor Optimizations (Readability, Nits)
- ...

5. Review the feedback written so far. Is the feedback comprehensive and sufficiently detailed? If not, go back to step 2.
6. Output the complete review.
```

### Example Tool Call

**Note**: When invoking the subagent via JSON tool call, ensure that any multi-line strings (like the prompt and the git diff) are properly escaped (e.g., replacing real newlines with `\n` and escaping quotes).

```json
{
  "toolSummary": "Invoke senior reviewer subagent",
  "toolAction": "Invoking subagent",
  "Subagents": [
    {
      "TypeName": "self",
      "Role": "Senior Code Reviewer",
      "Prompt": "<Insert Full Senior Reviewer Prompt from section above>\n\n[Insert git diff here]"
    }
  ]
}
```

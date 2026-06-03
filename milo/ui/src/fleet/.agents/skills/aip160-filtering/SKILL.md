---
name: aip160-filtering
description: Guidelines for generating AIP-160 filter strings for Fleet Console. Use when you need to generate filter strings for RPC queries or update the URL with filters.
---

# AIP-160 Filtering Skill

> **Note**: This document contains instructions for AI code assistants working in this repository. Human developers can use it as a reference.

Use this skill when you need to generate filter strings for RPC queries or update the URL with filters.

## Reference Documents

- **AIP-160 Filtering Architecture**: Refer to [AIP-160 Filtering Architecture](../../../docs/decisions/aip160-filtering.md) for the full decision doc on filter handling and transition states.

## Golden Examples

- **Single Value**: `labels."dut_state" = "ready"`
- **Multi-Value (Expanded)**: `(labels."label-pool" = "labstation_tryjob" OR labels."label-pool" = "labstation_main")`

## Rules

1. **Quoted Keys**: In production, the backend requires quotes around the key part for label fields (e.g., `labels."dut_state"`).
2. **Expanded OR**: For some fields (like `label-pool`), the backend requires the expanded `OR` syntax rather than shorthand.
3. **Existing Practices**: Work with existing coding practices (like hoisted state, shared hooks, and centralized constants) when possible. Do not reinvent state management or hardcode values that belong in shared files.

## Architecture

- Always hoist state to the page level and pass `aip160` string and `setFiltersBatch` down as props.
- Do not read from URL directly in child components.

## Gotchas

- The backend fails if quotes are missing around keys in label fields.
- The backend might not support shorthand OR syntax for some fields, so always default to expanded OR syntax when in doubt.

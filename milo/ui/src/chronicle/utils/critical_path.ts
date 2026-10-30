// Copyright 2026 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

import { parseTimestamp } from '@/chronicle/utils/time_utils';
import { Actor } from '@/proto/turboci/graph/orchestrator/v1/actor.pb';
import { Check } from '@/proto/turboci/graph/orchestrator/v1/check.pb';
import { CheckState } from '@/proto/turboci/graph/orchestrator/v1/check_state.pb';
import {
  Dependencies,
  Dependencies_Group,
} from '@/proto/turboci/graph/orchestrator/v1/dependencies.pb';
import { Resolution } from '@/proto/turboci/graph/orchestrator/v1/edge.pb';
import { Stage } from '@/proto/turboci/graph/orchestrator/v1/stage.pb';
import { StageState } from '@/proto/turboci/graph/orchestrator/v1/stage_state.pb';

/**
 * A workplan node (`Stage` or `Check`) paired with the historical lifecycle
 * state being resolved at the current step of the critical path walk.
 *
 * Note that `stage.state` and `check.state` on the protobuf objects only store
 * each node's latest state in the workplan (e.g. `FINAL` for all completed
 * nodes), whereas `GraphNodeState.state` tracks which specific state transition
 * of that node is on the critical path.
 *
 * Tracking `(node, state)` rather than bare node IDs also allows the critical
 * path to traverse the same node at two different states without treating it as
 * a cycle. For example:
 * - Stage `S6` depends on Check `C5` reaching `CHECK_STATE_FINAL`.
 * - `C5` is finalized by Stage `S5`.
 * - `S5` itself depends on `C5` reaching `CHECK_STATE_WAITING` (which happens
 *   once Stages `S3` and `S4` finish planning `C5`).
 * Here the critical path walks `S6 -> (C5, FINAL) -> S5 -> (C5, WAITING) -> S4`,
 * visiting `C5` twice at distinct states while still terminating if the exact
 * same `(node, state)` is revisited.
 */
export type GraphNodeState =
  | {
      readonly stage: Stage;
      readonly check?: never;
      readonly state: StageState;
    }
  | {
      readonly check: Check;
      readonly stage?: never;
      readonly state: CheckState;
    };

/**
 * The ordered `(node, state)` transitions and stage/check identifier sets lying
 * on the critical path of a workplan.
 */
export interface CriticalPathResult {
  /** Ordered sequence of `(node, state)` transitions from start to finish. */
  readonly path: readonly GraphNodeState[];
  readonly stageIds: ReadonlySet<string>;
  readonly checkIds: ReadonlySet<string>;
}

interface ResolvedBlocker {
  readonly node: GraphNodeState;
  readonly unblockedAtMs: number;
}

function toMs(ts: string | undefined): number | undefined {
  return parseTimestamp(ts)?.toMillis();
}

function getNodeId(node: GraphNodeState): string | undefined {
  return node.stage ? node.stage.identifier?.id : node.check.identifier?.id;
}

/**
 * Derives the timestamp in milliseconds when `stage` reached `targetState`.
 */
function getStageTimeMs(
  stage: Stage,
  targetState: StageState = StageState.STAGE_STATE_FINAL,
): number {
  const entry = stage.stateHistory.find((e) => e.state === targetState);
  return toMs(entry?.version?.ts) ?? 0;
}

/**
 * Derives the timestamp in milliseconds when `check` reached `targetState`
 * from its `stateHistory` or `edits`.
 */
function getCheckTimeMs(
  check: Check,
  targetState: CheckState = CheckState.CHECK_STATE_FINAL,
): number {
  const entry = check.stateHistory.find((e) => e.state === targetState);
  const edit = check.edits.find((e) => e.check?.state === targetState);
  return toMs(entry?.version?.ts ?? edit?.version?.ts) ?? 0;
}

/**
 * Resolves the parent `Stage` that dynamically spawned a `Stage` or `Check`
 * (recorded on `stage.createdBy` or `check.createdBy`).
 *
 * Because `stage.state` on the proto only stores the stage's latest state in
 * the workplan, the creator stage is paired with `STAGE_STATE_ATTEMPTING` — the
 * lifecycle state in which a stage executes its attempt and spawns child nodes.
 */
function getCreatorStageNode(
  createdBy: Actor | undefined,
  stageById: ReadonlyMap<string, Stage>,
): GraphNodeState | undefined {
  const id = createdBy?.stageAttempt?.stage?.id;
  const stage = id ? stageById.get(id) : undefined;
  return stage
    ? { stage, state: StageState.STAGE_STATE_ATTEMPTING }
    : undefined;
}

/**
 * Evaluates a `Dependencies` predicate tree and returns the upstream
 * `(node, state)` that last held it up, along with the timestamp at which that
 * blocker resolved.
 *
 * In TurboCI, `Dependencies` consists of:
 * - `edges`: flat list of `Stage` or `Check` dependency targets (with optional
 *   `condition.onState`, defaulting to `FINAL`).
 * - `resolutionEvents`: map from edge index to the `ResolutionEvent` recorded
 *   when the orchestrator resolved that edge (`RESOLUTION_SATISFIED` vs
 *   `RESOLUTION_UNSATISFIED`, plus `conditionVersion.ts`).
 * - `predicate`: root `Dependencies_Group` of a recursive tree where each
 *   group contains `edges` (indices into `Dependencies.edges`), `groups`
 *   (nested child `Dependencies_Group`s), and an optional `threshold`:
 *   - Unset `threshold` (`AND` group): all children (`edges` and `groups`)
 *     must resolve, so the last (slowest) child to resolve is the blocker.
 *   - `threshold: 1` (`OR` group): unblocks as soon as the 1st (earliest)
 *     satisfied child resolves.
 *   - `threshold: k` ($k$-of-$n$ group): unblocks when the $k$-th earliest
 *     satisfied child resolves.
 */
function evaluateDependencies(
  dependencies: Dependencies | undefined,
  current: GraphNodeState,
  stageById: ReadonlyMap<string, Stage>,
  checkById: ReadonlyMap<string, Check>,
): ResolvedBlocker | undefined {
  if (!dependencies || dependencies.edges.length === 0) {
    return undefined;
  }

  // Resolves a single edge by index in `dependencies.edges`.
  const evaluateEdge = (index: number): ResolvedBlocker | undefined => {
    const edge = dependencies.edges[index];
    const event = dependencies.resolutionEvents[index];
    // Ignore edges that were not satisfied (e.g. unchosen branches of an OR).
    if (
      !edge ||
      (event && event.resolution !== Resolution.RESOLUTION_SATISFIED)
    ) {
      return undefined;
    }

    // `conditionVersion` is when the target node met the condition; `version`
    // is when the dependent node observed it. If neither is recorded, fall
    // back to when the target node reached the required state.
    const satisfiedAtMs = toMs(
      event?.conditionVersion?.ts ?? event?.version?.ts,
    );

    if (edge.stage) {
      const stageId = edge.stage.identifier?.id;
      const stage = stageId ? stageById.get(stageId) : undefined;
      const state =
        edge.stage.condition?.onState || StageState.STAGE_STATE_FINAL;
      // Skip missing stages and self-referential edges to the same `(stage, state)`.
      if (
        !stage ||
        (current.stage?.identifier?.id === stageId && current.state === state)
      ) {
        return undefined;
      }
      return {
        node: { stage, state },
        unblockedAtMs: satisfiedAtMs ?? getStageTimeMs(stage, state),
      };
    }

    if (edge.check) {
      const checkId = edge.check.identifier?.id;
      const check = checkId ? checkById.get(checkId) : undefined;
      const state =
        edge.check.condition?.onState || CheckState.CHECK_STATE_FINAL;
      // Skip missing checks and self-referential edges to the same `(check, state)`.
      if (
        !check ||
        (current.check?.identifier?.id === checkId && current.state === state)
      ) {
        return undefined;
      }
      return {
        node: { check, state },
        unblockedAtMs: satisfiedAtMs ?? getCheckTimeMs(check, state),
      };
    }

    return undefined;
  };

  // Recursively evaluates a `Dependencies_Group` and returns the child blocker
  // that caused the group to cross its satisfaction threshold.
  const evaluateGroup = (
    group: Dependencies_Group,
  ): ResolvedBlocker | undefined => {
    const resolved: ResolvedBlocker[] = [];

    for (const edgeIndex of group.edges) {
      const blocker = evaluateEdge(edgeIndex);
      if (blocker) {
        resolved.push(blocker);
      }
    }

    for (const subgroup of group.groups) {
      const blocker = evaluateGroup(subgroup);
      if (blocker) {
        resolved.push(blocker);
      }
    }

    const totalChildren = group.edges.length + group.groups.length;
    const required = group.threshold || totalChildren;
    if (resolved.length === 0 || resolved.length < required) {
      return undefined;
    }

    // Sort satisfied children chronologically from earliest to latest.
    resolved.sort((a, b) => a.unblockedAtMs - b.unblockedAtMs);

    // Pick the child that completed the group's threshold:
    // - threshold = 1 (OR): index 0 (earliest satisfied child)
    // - threshold = k (k-of-n): index k - 1 (k-th earliest satisfied child)
    // - threshold unset (AND): index resolved.length - 1 (latest satisfied child)
    return resolved[
      group.threshold ? group.threshold - 1 : resolved.length - 1
    ];
  };

  // If `predicate` is omitted, treat all edges in `dependencies.edges` as a
  // single flat AND group.
  const rootGroup = dependencies.predicate ?? {
    edges: dependencies.edges.map((_, i) => i),
    groups: [],
  };

  return evaluateGroup(rootGroup);
}

/**
 * Finds the slowest `Stage` that edited `check` into `targetState`
 * (`CHECK_STATE_PLANNED` or `CHECK_STATE_FINAL`).
 *
 * When multiple stages contributed edits for `targetState`, returns the stage
 * whose edit occurred last.
 */
function findCheckEditor(
  check: Check,
  targetState: CheckState,
  stageById: ReadonlyMap<string, Stage>,
): GraphNodeState | undefined {
  let latestStage: Stage | undefined;
  let latestMs = -Infinity;

  for (const edit of check.edits) {
    if (edit.check?.state !== targetState) continue;

    const stageId = edit.createdBy?.stageAttempt?.stage?.id;
    const stage = stageId ? stageById.get(stageId) : undefined;
    if (!stage) continue;

    const timeMs = toMs(edit.version?.ts) ?? getStageTimeMs(stage);
    if (timeMs > latestMs) {
      latestMs = timeMs;
      latestStage = stage;
    }
  }

  return latestStage
    ? {
        stage: latestStage,
        state: StageState.STAGE_STATE_FINAL,
      }
    : undefined;
}

const STAGE_STATES_IN_ORDER: readonly StageState[] = [
  StageState.STAGE_STATE_PLANNED,
  StageState.STAGE_STATE_ATTEMPTING,
  StageState.STAGE_STATE_AWAITING_GROUP,
  StageState.STAGE_STATE_FINAL,
];

const CHECK_STATES_IN_ORDER: readonly CheckState[] = [
  CheckState.CHECK_STATE_PLANNING,
  CheckState.CHECK_STATE_PLANNED,
  CheckState.CHECK_STATE_WAITING,
  CheckState.CHECK_STATE_FINAL,
];

/**
 * Finds the upstream `(node, state)` that directly blocked `stage` from
 * reaching `state`, or `undefined` if `stage` did not wait on another node at
 * `state` (in which case the caller steps back to the prior state).
 *
 * Stage lifecycle (`PLANNED -> ATTEMPTING -> AWAITING_GROUP -> FINAL`):
 * - `PLANNED`: Newly created stage; returns the parent stage attempt in
 *   `stage.createdBy`.
 * - `ATTEMPTING`: Reached once `stage.dependencies` resolve.
 * - `AWAITING_GROUP`: Entered as soon as the stage's own attempts finish; no
 *   external dependency blocks entering this state.
 * - `FINAL`: Reached once `stage.continuationGroup` resolves (if the stage
 *   waited on one in `AWAITING_GROUP`).
 */
function findStageBlocker(
  stage: Stage,
  state: StageState,
  stageById: ReadonlyMap<string, Stage>,
  checkById: ReadonlyMap<string, Check>,
): GraphNodeState | undefined {
  const current: GraphNodeState = { stage, state };

  switch (state) {
    case StageState.STAGE_STATE_PLANNED:
      return getCreatorStageNode(stage.createdBy, stageById);

    case StageState.STAGE_STATE_ATTEMPTING:
      return evaluateDependencies(
        stage.dependencies,
        current,
        stageById,
        checkById,
      )?.node;

    case StageState.STAGE_STATE_FINAL: {
      const contBlocker = evaluateDependencies(
        stage.continuationGroup,
        current,
        stageById,
        checkById,
      );
      if (
        contBlocker &&
        contBlocker.unblockedAtMs >=
          getStageTimeMs(stage, StageState.STAGE_STATE_AWAITING_GROUP)
      ) {
        return contBlocker.node;
      }
      return undefined;
    }

    default:
      return undefined;
  }
}

/**
 * Finds the upstream `(node, state)` that directly blocked `check` from
 * reaching `state`, or `undefined` if `check` did not wait on another node at
 * `state` (in which case the caller steps back to the prior state).
 *
 * Check lifecycle (`PLANNING -> PLANNED -> WAITING -> FINAL`):
 * - `PLANNING`: Newly created check; returns the parent stage attempt in
 *   `check.createdBy`.
 * - `PLANNED`: Options and dependencies are locked by the last stage edit
 *   that transitioned `check` to `CHECK_STATE_PLANNED`.
 * - `WAITING`: Reached once `check.dependencies` resolve.
 * - `FINAL`: Results are finalized by the last stage edit that transitioned
 *   `check` to `CHECK_STATE_FINAL`.
 */
function findCheckBlocker(
  check: Check,
  state: CheckState,
  stageById: ReadonlyMap<string, Stage>,
  checkById: ReadonlyMap<string, Check>,
): GraphNodeState | undefined {
  const current: GraphNodeState = { check, state };

  switch (state) {
    case CheckState.CHECK_STATE_PLANNING:
      return getCreatorStageNode(check.createdBy, stageById);

    case CheckState.CHECK_STATE_PLANNED:
      return findCheckEditor(check, CheckState.CHECK_STATE_PLANNED, stageById);

    case CheckState.CHECK_STATE_WAITING:
      return evaluateDependencies(
        check.dependencies,
        current,
        stageById,
        checkById,
      )?.node;

    case CheckState.CHECK_STATE_FINAL:
      return findCheckEditor(check, CheckState.CHECK_STATE_FINAL, stageById);

    default:
      return undefined;
  }
}

/**
 * Finds the blocking predecessor `(node, state)` that last held up `current`.
 *
 * Starting at `current.state`, checks what directly blocked that state. If no
 * blocker held up `current` at that state (for example, `FINAL` with no
 * `continuationGroup`, or `WAITING` with no `dependencies`), steps backward to
 * the preceding lifecycle state on the same node and tries again.
 */
function findBlockingPredecessor(
  current: GraphNodeState,
  stageById: ReadonlyMap<string, Stage>,
  checkById: ReadonlyMap<string, Check>,
): GraphNodeState | undefined {
  if (current.stage) {
    for (let i = STAGE_STATES_IN_ORDER.indexOf(current.state); i >= 0; i--) {
      const blocker = findStageBlocker(
        current.stage,
        STAGE_STATES_IN_ORDER[i],
        stageById,
        checkById,
      );
      if (blocker) {
        return blocker;
      }
    }
    return undefined;
  }

  for (let i = CHECK_STATES_IN_ORDER.indexOf(current.state); i >= 0; i--) {
    const blocker = findCheckBlocker(
      current.check,
      CHECK_STATES_IN_ORDER[i],
      stageById,
      checkById,
    );
    if (blocker) {
      return blocker;
    }
  }
  return undefined;
}

/**
 * Determines the critical path through a workplan's stages and checks.
 *
 * Starting from the last node to finish, this steps backward over
 * `(node, state)` pairs across stage/check dependencies (including AND, OR,
 * k-of-n thresholds, and nested predicate groups) and check edits.
 */
export function computeCriticalPath(
  stages: readonly Stage[],
  checks: readonly Check[] = [],
): CriticalPathResult {
  const reversedPath: GraphNodeState[] = [];
  const stageIds = new Set<string>();
  const checkIds = new Set<string>();
  const stageById = new Map<string, Stage>();
  const checkById = new Map<string, Check>();

  let current: GraphNodeState | undefined;
  let maxEndMs = -Infinity;

  for (const stage of stages) {
    const id = stage.identifier?.id;
    if (!id) continue;
    stageById.set(id, stage);

    const endMs = getStageTimeMs(stage);
    if (endMs > maxEndMs) {
      maxEndMs = endMs;
      current = {
        stage,
        state: stage.state || StageState.STAGE_STATE_FINAL,
      };
    }
  }

  for (const check of checks) {
    const id = check.identifier?.id;
    if (!id) continue;
    checkById.set(id, check);

    const endMs = getCheckTimeMs(check);
    if (endMs > maxEndMs) {
      maxEndMs = endMs;
      current = {
        check,
        state: check.state || CheckState.CHECK_STATE_FINAL,
      };
    }
  }

  const visitedStageStates = new Set<string>();
  const visitedCheckStates = new Set<string>();
  while (current) {
    const id = getNodeId(current);
    const visited = current.stage ? visitedStageStates : visitedCheckStates;
    const stateKey = `${id ?? ''}:${current.state}`;
    if (!id || visited.has(stateKey)) {
      break;
    }
    visited.add(stateKey);
    reversedPath.push(current);
    (current.stage ? stageIds : checkIds).add(id);

    current = findBlockingPredecessor(current, stageById, checkById);
  }

  return { path: reversedPath.reverse(), stageIds, checkIds };
}

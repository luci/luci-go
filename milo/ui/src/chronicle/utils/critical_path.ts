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
import { Resolution } from '@/proto/turboci/graph/orchestrator/v1/edge.pb';
import { Stage } from '@/proto/turboci/graph/orchestrator/v1/stage.pb';
import { StageState } from '@/proto/turboci/graph/orchestrator/v1/stage_state.pb';

/**
 * Derives the end timestamp in milliseconds of a stage from its state history.
 */
function getStageEndTimeMs(stage: Stage): number {
  const finalEntry =
    stage.stateHistory?.find((e) => e.state === StageState.STAGE_STATE_FINAL) ??
    stage.stateHistory?.[stage.stateHistory.length - 1];
  return parseTimestamp(finalEntry?.version?.ts)?.toMillis() ?? 0;
}

/**
 * Finds the dependency stage that last unblocked `stage` (the predecessor that
 * held `stage` up).
 *
 * Edges pointing at stages outside `stageById` (for example stages that never
 * reached a final state, and so are absent from the timeline) are skipped.
 */
function findBlockingPredecessor(
  stage: Stage,
  stageById: ReadonlyMap<string, Stage>,
  endMsByStageId: ReadonlyMap<string, number>,
): Stage | undefined {
  const dependencies = stage.dependencies;
  if (!dependencies) {
    return undefined;
  }

  const events = dependencies.resolutionEvents;
  let blocker: Stage | undefined;
  let blockerUnblockedAtMs = -Infinity;

  for (const [index, edge] of (dependencies.edges ?? []).entries()) {
    const event = events?.[index];
    // Skip dependencies that did not resolve successfully.
    if (event && event.resolution !== Resolution.RESOLUTION_SATISFIED) {
      continue;
    }

    const stageId = edge.stage?.identifier?.id;
    const predecessor = stageId ? stageById.get(stageId) : undefined;
    if (!predecessor || predecessor.identifier?.id === stage.identifier?.id) {
      continue;
    }

    // `conditionVersion` is when the edge's condition was actually met, while
    // `version` is when this stage observed it, which can be much later. When
    // neither is recorded, fall back to when the predecessor itself finished.
    const satisfiedAt = parseTimestamp(
      event?.conditionVersion?.ts ?? event?.version?.ts,
    );
    const unblockedAtMs =
      satisfiedAt?.toMillis() ?? endMsByStageId.get(stageId!) ?? 0;

    if (unblockedAtMs > blockerUnblockedAtMs) {
      blocker = predecessor;
      blockerUnblockedAtMs = unblockedAtMs;
    }
  }

  return blocker;
}

/**
 * Determines the critical path through a set of stages.
 *
 * Starting from the last stage to finish, this repeatedly steps back to
 * whichever of its dependencies was satisfied last, since that is the one that
 * actually held the stage up. The walk stops when a stage has no resolvable
 * predecessors.
 *
 * @param stages The stages to consider.
 * @returns The ids of the stages lying on the critical path.
 */
export function computeCriticalPath(
  stages: readonly Stage[],
): ReadonlySet<string> {
  const path = new Set<string>();
  if (stages.length === 0) {
    return path;
  }

  const stageById = new Map<string, Stage>();
  const endMsByStageId = new Map<string, number>();

  for (const stage of stages) {
    const id = stage.identifier?.id;
    if (id) {
      stageById.set(id, stage);
      endMsByStageId.set(id, getStageEndTimeMs(stage));
    }
  }

  let current: Stage | undefined;
  let maxEndMs = -Infinity;
  for (const stage of stages) {
    const id = stage.identifier?.id;
    const endMs = id ? (endMsByStageId.get(id) ?? -Infinity) : -Infinity;
    if (endMs > maxEndMs) {
      maxEndMs = endMs;
      current = stage;
    }
  }

  // Revisiting a stage would mean the dependency graph contains a cycle.
  while (
    current &&
    current.identifier?.id &&
    !path.has(current.identifier.id)
  ) {
    path.add(current.identifier.id);
    current = findBlockingPredecessor(current, stageById, endMsByStageId);
  }
  return path;
}

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

/**
 * Utilities for deriving Stage -> Check assignments from Check dependency
 * edges:
 *
 *   * Check edges with `on_state` `PLANNING` map to `EDITING` and `WAITING`
 *     map to `EXECUTION`; all other Check edges remain plain dependencies.
 *   * Legacy `WorkNodeBridge` stages (`TYPE_URL_WORKNODE_BRIDGE`) are excluded
 *     from derived assignments because they only bridge Check readiness to
 *     downstream legacy `WorkNode` stages without writing Check results.
 */

import { Check as CheckId } from '@/proto/turboci/graph/ids/v1/identifier.pb';
import { CheckState } from '@/proto/turboci/graph/orchestrator/v1/check_state.pb';
import { Edge } from '@/proto/turboci/graph/orchestrator/v1/edge.pb';
import { Stage } from '@/proto/turboci/graph/orchestrator/v1/stage.pb';

import { TYPE_URL_WORKNODE_BRIDGE } from './legacy_worknode';

export enum AssignmentType {
  EDITING = 'EDITING',
  EXECUTION = 'EXECUTION',
}

export interface StageAssignment {
  readonly checkId: string;
  readonly target: CheckId;
  readonly type: AssignmentType;
}

function assignmentTypeFromOnState(
  onState: CheckState | undefined,
): AssignmentType | undefined {
  switch (onState) {
    case CheckState.CHECK_STATE_PLANNING:
      return AssignmentType.EDITING;
    case CheckState.CHECK_STATE_WAITING:
      return AssignmentType.EXECUTION;
    default:
      return undefined;
  }
}

/** Returns true if `edge` is a derived Check assignment edge for `stage`. */
export function isStageAssignmentEdge(stage: Stage, edge: Edge): boolean {
  if (
    stage.args?.typeUrl === TYPE_URL_WORKNODE_BRIDGE ||
    !edge.check?.identifier?.id
  ) {
    return false;
  }
  return assignmentTypeFromOnState(edge.check.condition?.onState) !== undefined;
}

/**
 * Returns the Checks `stage` works on (`EXECUTION` overriding `EDITING`),
 * sorted `EDITING` before `EXECUTION` and then by Check ID.
 */
export function getStageAssignments(stage: Stage): StageAssignment[] {
  const assignmentByCheckId = new Map<string, StageAssignment>();

  if (stage.args?.typeUrl !== TYPE_URL_WORKNODE_BRIDGE) {
    for (const edge of stage.dependencies?.edges ?? []) {
      const target = edge.check?.identifier;
      if (!target?.id) continue;
      const type = assignmentTypeFromOnState(edge.check?.condition?.onState);
      if (
        !type ||
        assignmentByCheckId.get(target.id)?.type === AssignmentType.EXECUTION
      ) {
        continue;
      }
      assignmentByCheckId.set(target.id, { checkId: target.id, target, type });
    }
  }

  // TODO(b/568863884): Read stored assignments once the orchestrator stores
  // them.

  return Array.from(assignmentByCheckId.values()).sort(compareAssignments);
}

function compareAssignments(a: StageAssignment, b: StageAssignment): number {
  if (a.type !== b.type) {
    return a.type === AssignmentType.EDITING ? -1 : 1;
  }
  return a.checkId < b.checkId ? -1 : a.checkId > b.checkId ? 1 : 0;
}

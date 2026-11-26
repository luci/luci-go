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

import { CheckState } from '@/proto/turboci/graph/orchestrator/v1/check_state.pb';
import { Edge } from '@/proto/turboci/graph/orchestrator/v1/edge.pb';
import { Stage } from '@/proto/turboci/graph/orchestrator/v1/stage.pb';

import {
  AssignmentType,
  getStageAssignments,
  isStageAssignmentEdge,
} from './assignment_utils';
import { TYPE_URL_WORKNODE_BRIDGE } from './legacy_worknode';

const WORK_PLAN = { id: 'wp' };

function checkEdge(id: string, onState?: CheckState): Edge {
  return {
    check: {
      identifier: { workPlan: WORK_PLAN, id },
      condition: onState === undefined ? undefined : { onState },
    },
  };
}

function stageEdge(id: string): Edge {
  return { stage: { identifier: { workPlan: WORK_PLAN, id } } };
}

describe('assignment_utils', () => {
  describe('isStageAssignmentEdge', () => {
    const normalStage = Stage.fromPartial({
      identifier: { id: 'manage-build-1', isWorknode: false },
      args: { typeUrl: 'type.googleapis.com/example.ManageTargetBuildArgs' },
    });
    const bridgeStage = Stage.fromPartial({
      identifier: { id: 'bridge-1', isWorknode: true },
      args: { typeUrl: TYPE_URL_WORKNODE_BRIDGE },
    });

    it.each<CheckState>([
      CheckState.CHECK_STATE_PLANNING,
      CheckState.CHECK_STATE_WAITING,
    ])('classifies %s check edge as assignment', (onState) => {
      expect(isStageAssignmentEdge(normalStage, checkEdge('C1', onState))).toBe(
        true,
      );
    });

    it.each<readonly [string, Edge]>([
      ['default check edge', checkEdge('C1')],
      ['UNKNOWN check edge', checkEdge('C1', CheckState.CHECK_STATE_UNKNOWN)],
      ['PLANNED check edge', checkEdge('C1', CheckState.CHECK_STATE_PLANNED)],
      ['FINAL check edge', checkEdge('C1', CheckState.CHECK_STATE_FINAL)],
      ['stage edge', stageEdge('S1')],
      ['empty edge', {}],
      [
        'check edge missing identifier',
        { check: { condition: { onState: CheckState.CHECK_STATE_WAITING } } },
      ],
    ])('rejects %s', (_, edge) => {
      expect(isStageAssignmentEdge(normalStage, edge)).toBe(false);
    });

    it.each<CheckState>([
      CheckState.CHECK_STATE_PLANNING,
      CheckState.CHECK_STATE_WAITING,
    ])('excludes WorkNodeBridge stage for %s check edge', (onState) => {
      expect(isStageAssignmentEdge(bridgeStage, checkEdge('C1', onState))).toBe(
        false,
      );
    });
  });

  describe('getStageAssignments', () => {
    it('returns an empty list for a stage without edges', () => {
      const stage = Stage.fromPartial({ identifier: { id: 'S1' } });
      expect(getStageAssignments(stage)).toEqual([]);
    });

    it('derives assignments from PLANNING and WAITING edges and ignores other edges', () => {
      const stage = Stage.fromPartial({
        identifier: { id: 'S1' },
        dependencies: {
          edges: [
            checkEdge('C_final', CheckState.CHECK_STATE_FINAL),
            checkEdge('C_unknown', CheckState.CHECK_STATE_UNKNOWN),
            checkEdge('C_default'),
            {
              check: { condition: { onState: CheckState.CHECK_STATE_WAITING } },
            },
            checkEdge('C_waiting', CheckState.CHECK_STATE_WAITING),
            checkEdge('C_planned', CheckState.CHECK_STATE_PLANNED),
            checkEdge('C_edit', CheckState.CHECK_STATE_PLANNING),
            stageEdge('S0'),
          ],
        },
      });

      expect(getStageAssignments(stage)).toEqual([
        {
          checkId: 'C_edit',
          target: { workPlan: WORK_PLAN, id: 'C_edit' },
          type: AssignmentType.EDITING,
        },
        {
          checkId: 'C_waiting',
          target: { workPlan: WORK_PLAN, id: 'C_waiting' },
          type: AssignmentType.EXECUTION,
        },
      ]);
    });

    it('skips derived check edges for WorkNodeBridge stages', () => {
      const bridge = Stage.fromPartial({
        identifier: { id: 'bridge-1', isWorknode: true },
        args: { typeUrl: TYPE_URL_WORKNODE_BRIDGE },
        dependencies: {
          edges: [checkEdge('C_build', CheckState.CHECK_STATE_WAITING)],
        },
      });
      expect(getStageAssignments(bridge)).toEqual([]);
    });

    it.each<readonly [string, Edge[]]>([
      [
        'WAITING then PLANNING',
        [
          checkEdge('C1', CheckState.CHECK_STATE_WAITING),
          checkEdge('C1', CheckState.CHECK_STATE_PLANNING),
        ],
      ],
      [
        'PLANNING then WAITING',
        [
          checkEdge('C1', CheckState.CHECK_STATE_PLANNING),
          checkEdge('C1', CheckState.CHECK_STATE_WAITING),
        ],
      ],
    ])('reports a stage with %s edges on a check as EXECUTION', (_, edges) => {
      const stage = Stage.fromPartial({
        identifier: { id: 'S_dup' },
        dependencies: { edges },
      });

      expect(
        getStageAssignments(stage).map((a) => [a.checkId, a.type]),
      ).toEqual([['C1', AssignmentType.EXECUTION]]);
    });

    it('orders assignments by type (EDITING first) and then by check id', () => {
      const stage = Stage.fromPartial({
        identifier: { id: 'S1' },
        dependencies: {
          edges: [
            checkEdge('Z_exec', CheckState.CHECK_STATE_WAITING),
            checkEdge('B_edit', CheckState.CHECK_STATE_PLANNING),
            checkEdge('A_exec', CheckState.CHECK_STATE_WAITING),
            checkEdge('A_edit', CheckState.CHECK_STATE_PLANNING),
          ],
        },
      });

      expect(getStageAssignments(stage).map((a) => a.checkId)).toEqual([
        'A_edit',
        'B_edit',
        'A_exec',
        'Z_exec',
      ]);
    });
  });
});

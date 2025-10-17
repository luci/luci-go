// Copyright 2025 The LUCI Authors.
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

import { DeepPartial } from '@/proto/turboci/data/build/v1/build_check_options.pb';
import {
  Check as CheckId,
  Stage as StageId,
  WorkPlan,
} from '@/proto/turboci/graph/ids/v1/identifier.pb';
import { Check } from '@/proto/turboci/graph/orchestrator/v1/check.pb';
import { CheckDelta } from '@/proto/turboci/graph/orchestrator/v1/check_delta.pb';
import { CheckEditView } from '@/proto/turboci/graph/orchestrator/v1/check_edit_view.pb';
import { CheckKind } from '@/proto/turboci/graph/orchestrator/v1/check_kind.pb';
import { CheckState } from '@/proto/turboci/graph/orchestrator/v1/check_state.pb';
import { CheckView } from '@/proto/turboci/graph/orchestrator/v1/check_view.pb';
import { EdgeGroup } from '@/proto/turboci/graph/orchestrator/v1/edge_group.pb';
import { Edit } from '@/proto/turboci/graph/orchestrator/v1/edit.pb';
import { GraphView as TurboCIGraphView } from '@/proto/turboci/graph/orchestrator/v1/graph_view.pb';
import { Stage } from '@/proto/turboci/graph/orchestrator/v1/stage.pb';
import { StageView } from '@/proto/turboci/graph/orchestrator/v1/stage_view.pb';

import { convertTurboCIGraphToNodesAndEdges } from './graph_utils';

const WORKPLAN: WorkPlan = { id: 'workplan-1' };

// Helper to create IDs for manual graph construction in tests.
function createCheckId(id: string): CheckId {
  return { workPlan: WORKPLAN, id };
}
function createStageId(id: string): StageId {
  return { workPlan: WORKPLAN, id };
}

// Helper to create a minimal valid Check object for tests
function createCheck(params: DeepPartial<Check>): Check {
  return {
    dependencies: [],
    options: [],
    results: [],
    ...params,
  } as Check;
}

// Helper to create a minimal valid CheckView object for tests
function createCheckView(params: DeepPartial<CheckView>): CheckView {
  return {
    optionData: [],
    edits: [],
    results: [],
    ...params,
  } as CheckView;
}

// Helper to create a minimal valid Stage object for tests
function createStage(params: DeepPartial<Stage>): Stage {
  return {
    dependencies: [],
    attempts: [],
    assignments: [],
    continuationGroup: [],
    ...params,
  } as Stage;
}

// Helper to create a minimal valid StageView object for tests
function createStageView(params: DeepPartial<StageView>): StageView {
  return {
    edits: [],
    ...params,
  } as StageView;
}

// Helper to create a minimal valid CheckDelta for tests
function createCheckDelta(params: DeepPartial<CheckDelta>): CheckDelta {
  return {
    dependencies: [],
    options: [],
    result: [],
    ...params,
  } as CheckDelta;
}

// Helper to create a minimal valid Edit for tests
function createEdit(params: DeepPartial<Edit>): Edit {
  return {
    transactionalSet: [],
    reasons: [],
    ...params,
  } as Edit;
}

// Helper to create a minimal valid CheckEditView for tests
function createCheckEditView(
  params: DeepPartial<CheckEditView>,
): CheckEditView {
  return {
    optionData: [],
    ...params,
  } as CheckEditView;
}

// Helper to create a minimal valid EdgeGroup for tests
function createEdgeGroup(params: DeepPartial<EdgeGroup>): EdgeGroup {
  return {
    edges: [],
    groups: [],
    ...params,
  } as EdgeGroup;
}

describe('graph_utils', () => {
  describe('convertTurboCIGraphToNodesAndEdges', () => {
    it('should create nodes and edges correctly for a simple graph', () => {
      // Manually construct a graph:
      // S1 depends on C1, C2
      // C3 depends on C2
      const c1Id = createCheckId('C1');
      const c2Id = createCheckId('C2');
      const c3Id = createCheckId('C3');
      const s1Id = createStageId('S1');

      const graph: TurboCIGraphView = {
        checks: [
          createCheckView({
            check: createCheck({
              identifier: c1Id,
              kind: CheckKind.CHECK_KIND_SOURCE,
            }),
          }),
          createCheckView({
            check: createCheck({
              identifier: c2Id,
              kind: CheckKind.CHECK_KIND_BUILD,
            }),
          }),
          createCheckView({
            check: createCheck({
              identifier: c3Id,
              kind: CheckKind.CHECK_KIND_TEST,
              dependencies: [
                createEdgeGroup({ edges: [{ target: { check: c2Id } }] }),
              ],
            }),
          }),
        ],
        stages: [
          createStageView({
            stage: createStage({
              identifier: s1Id,
              dependencies: [
                createEdgeGroup({
                  edges: [
                    { target: { check: c1Id } },
                    { target: { check: c2Id } },
                  ],
                }),
              ],
            }),
          }),
        ],
      };

      const { nodes, edges } = convertTurboCIGraphToNodesAndEdges(graph);

      expect(nodes).toHaveLength(4);
      expect(edges).toHaveLength(3);

      // Check nodes
      const nodeIds = nodes.map((n) => n.id);
      expect(nodeIds).toEqual(['S1', 'C1', 'C2', 'C3']);

      // Check edges
      const edgeIds = edges.map((e) => e.id);
      expect(edgeIds).toEqual([
        'dependency-S1-C1',
        'dependency-S1-C2',
        'dependency-C3-C2',
      ]);

      const edge1 = edges.find((e) => e.id === 'dependency-S1-C1');
      expect(edge1?.source).toBe('C1');
      expect(edge1?.target).toBe('S1');

      const edge2 = edges.find((e) => e.id === 'dependency-S1-C2');
      expect(edge2?.source).toBe('C2');
      expect(edge2?.target).toBe('S1');

      const edge3 = edges.find((e) => e.id === 'dependency-C3-C2');
      expect(edge3?.source).toBe('C2');
      expect(edge3?.target).toBe('C3');
    });

    it('should have the right node labels for different check kinds', () => {
      const graph: TurboCIGraphView = {
        checks: [
          createCheckView({
            check: createCheck({
              identifier: createCheckId('C1'),
              kind: CheckKind.CHECK_KIND_BUILD,
            }),
          }),
          createCheckView({
            check: createCheck({
              identifier: createCheckId('C2'),
              kind: CheckKind.CHECK_KIND_TEST,
            }),
          }),
          createCheckView({
            check: createCheck({
              identifier: createCheckId('C3'),
              kind: CheckKind.CHECK_KIND_SOURCE,
            }),
          }),
          createCheckView({
            check: createCheck({
              identifier: createCheckId('C4'),
              kind: CheckKind.CHECK_KIND_ANALYSIS,
            }),
          }),
        ],
        stages: [],
      };

      const { nodes } = convertTurboCIGraphToNodesAndEdges(graph);

      expect(nodes.find((n) => n.id === 'C1')?.data.label).toBe(
        'Build Check: C1',
      );
      expect(nodes.find((n) => n.id === 'C2')?.data.label).toBe(
        'Test Check: C2',
      );
      expect(nodes.find((n) => n.id === 'C3')?.data.label).toBe(
        'Source Check: C3',
      );
      expect(nodes.find((n) => n.id === 'C4')?.data.label).toBe(
        'Analysis Check: C4',
      );
    });

    it('should throw an error for an invalid check', () => {
      const invalidCheckGraph = {
        stages: [],
        checks: [{ check: {} }],
      };

      expect(() =>
        convertTurboCIGraphToNodesAndEdges(
          invalidCheckGraph as unknown as TurboCIGraphView,
        ),
      ).toThrow('Invalid CheckView: {"check":{}}');
    });

    it('should create edit edges based on latest state change', () => {
      // S1 edits C1.
      const c1Id = createCheckId('C1');
      const s1Id = createStageId('S1');

      const graph: TurboCIGraphView = {
        stages: [createStageView({ stage: createStage({ identifier: s1Id }) })],
        checks: [
          createCheckView({
            check: createCheck({
              identifier: c1Id,
              kind: CheckKind.CHECK_KIND_BUILD,
            }),
            edits: [
              createCheckEditView({
                edit: createEdit({
                  editor: { stageAttempt: { stage: s1Id } },
                  check: createCheckDelta({
                    state: CheckState.CHECK_STATE_PLANNING,
                  }),
                  version: { ts: '100' }, // Older
                }),
              }),
              createCheckEditView({
                edit: createEdit({
                  editor: { stageAttempt: { stage: s1Id } },
                  check: createCheckDelta({
                    state: CheckState.CHECK_STATE_FINAL,
                  }),
                  version: { ts: '200' }, // Newer
                }),
              }),
            ],
          }),
        ],
      };

      const { edges } = convertTurboCIGraphToNodesAndEdges(graph);

      // There should only be one edge for the FINAL state transition.
      expect(edges).toHaveLength(1);
      const editEdge = edges[0];

      // Check the edge details.
      expect(editEdge.id).toBe('edit-S1-C1');
      expect(editEdge.source).toBe('S1');
      expect(editEdge.target).toBe('C1');
      expect(editEdge.label).toBe('Final');
    });

    it('should create both dependency and edit edges between the same source/target nodes', () => {
      // C1 depends on S1.
      // S1 edits C1.
      const c1Id = createCheckId('C1');
      const s1Id = createStageId('S1');

      const graph: TurboCIGraphView = {
        stages: [createStageView({ stage: createStage({ identifier: s1Id }) })],
        checks: [
          createCheckView({
            check: createCheck({
              identifier: c1Id,
              kind: CheckKind.CHECK_KIND_BUILD,
              dependencies: [
                createEdgeGroup({ edges: [{ target: { stage: s1Id } }] }),
              ],
            }),
            edits: [
              createCheckEditView({
                edit: createEdit({
                  editor: { stageAttempt: { stage: s1Id } },
                  check: createCheckDelta({
                    state: CheckState.CHECK_STATE_FINAL,
                  }),
                  version: { ts: '200' },
                }),
              }),
            ],
          }),
        ],
      };

      const { edges } = convertTurboCIGraphToNodesAndEdges(graph);

      expect(edges).toHaveLength(2);
      const dependencyEdge = edges.find((e) => e.id === 'dependency-C1-S1');
      expect(dependencyEdge?.source).toBe('S1');
      expect(dependencyEdge?.target).toBe('C1');

      const editEdge = edges.find((e) => e.id === 'edit-S1-C1');
      expect(editEdge?.source).toBe('S1');
      expect(editEdge?.target).toBe('C1');
      expect(editEdge?.label).toBe('Final');
    });

    it('should throw an error for an invalid stage', () => {
      const invalidStageGraph = {
        stages: [{ stage: {} }],
      };

      expect(() =>
        convertTurboCIGraphToNodesAndEdges(
          invalidStageGraph as unknown as TurboCIGraphView,
        ),
      ).toThrow('Invalid StageView: {"stage":{}}');
    });
  });
});

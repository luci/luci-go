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

import {
  Check as CheckId,
  Identifier,
  Stage as StageId,
  WorkPlan,
} from '@/proto/turboci/graph/ids/v1/identifier.pb';
import { Check } from '@/proto/turboci/graph/orchestrator/v1/check.pb';
import { CheckKind } from '@/proto/turboci/graph/orchestrator/v1/check_kind.pb';
import { CheckView } from '@/proto/turboci/graph/orchestrator/v1/check_view.pb';
import { EdgeGroup } from '@/proto/turboci/graph/orchestrator/v1/edge_group.pb';
import { GraphView as TurboCIGraphView } from '@/proto/turboci/graph/orchestrator/v1/graph_view.pb';
import { Stage_Assignment } from '@/proto/turboci/graph/orchestrator/v1/stage.pb';
import { Stage } from '@/proto/turboci/graph/orchestrator/v1/stage.pb';
import { StageView } from '@/proto/turboci/graph/orchestrator/v1/stage_view.pb';

import { TurboCIGraphBuilder } from './graph_builder';

const WORKPLAN: WorkPlan = { id: 'test-plan' };

/** Creates a general Identifier pointing to a Check. */
function createCheckIdentifier(id: string): Identifier {
  return {
    check: { workPlan: WORKPLAN, id },
  };
}

/** Creates a general Identifier pointing to a Stage. */
function createStageIdentifier(id: string): Identifier {
  return {
    stage: { workPlan: WORKPLAN, id },
  };
}

/** Creates a partial CheckView for testing. */
function createCheckView(
  id: string,
  kind: CheckKind = CheckKind.CHECK_KIND_BUILD,
  // Dependencies are general Identifiers (can point to Check or Stage)
  dependencies: Identifier[] = [],
): CheckView {
  // simplify: create one edge group per dependency
  const edgeGroups: EdgeGroup[] = dependencies.map((dep) => ({
    edges: [{ target: dep }],
    groups: [],
    // other fields like threshold/resolution can be undefined for these tests
  }));

  const checkId: CheckId = { workPlan: WORKPLAN, id };

  return {
    check: {
      identifier: checkId,
      kind: kind,
      dependencies: edgeGroups,
      options: [],
      results: [],
      // other fields like realm/version/state can be undefined
    } as Check,
    edits: [],
    results: [],
    optionData: [],
  };
}

/** Creates a partial StageView for testing. */
function createStageView(
  id: string,
  // IDs of checks this stage is assigned to.
  assignedToCheckIds: string[] = [],
  // Dependencies are general Identifiers.
  dependencies: Identifier[] = [],
): StageView {
  const edgeGroups: EdgeGroup[] = dependencies.map((dep) => ({
    edges: [{ target: dep }],
    groups: [],
  }));

  const assignments: Stage_Assignment[] = assignedToCheckIds.map(
    (checkIdStr) => ({
      target: { workPlan: WORKPLAN, id: checkIdStr },
    }),
  );

  const stageId: StageId = { workPlan: WORKPLAN, id };

  return {
    stage: {
      identifier: stageId,
      dependencies: edgeGroups,
      assignments: assignments,
      attempts: [],
      continuationGroup: [],
      // other fields can be undefined
    } as Stage,
    edits: [],
  };
}

describe('TurboCIGraphBuilder', () => {
  it('should return empty arrays for an empty graph', () => {
    const graph: TurboCIGraphView = {
      checks: [],
      stages: [],
    };
    const builder = new TurboCIGraphBuilder(graph);
    const { nodes, edges } = builder.build();

    expect(nodes).toEqual([]);
    expect(edges).toEqual([]);
  });

  describe('Node Creation and Labeling', () => {
    it('should create nodes with correct labels based on CheckKind', () => {
      const graph: TurboCIGraphView = {
        checks: [
          createCheckView('C1', CheckKind.CHECK_KIND_SOURCE),
          createCheckView('C2', CheckKind.CHECK_KIND_BUILD),
          createCheckView('C3', CheckKind.CHECK_KIND_TEST),
          createCheckView('C4', CheckKind.CHECK_KIND_ANALYSIS),
          createCheckView('C5', CheckKind.CHECK_KIND_UNKNOWN),
        ],
        stages: [createStageView('S1')],
      };

      const { nodes } = new TurboCIGraphBuilder(graph).build();

      expect(nodes.find((n) => n.id === 'C1')?.data.label).toBe(
        'Source Check: C1',
      );
      expect(nodes.find((n) => n.id === 'C2')?.data.label).toBe(
        'Build Check: C2',
      );
      expect(nodes.find((n) => n.id === 'C3')?.data.label).toBe(
        'Test Check: C3',
      );
      expect(nodes.find((n) => n.id === 'C4')?.data.label).toBe(
        'Analysis Check: C4',
      );
      expect(nodes.find((n) => n.id === 'C5')?.data.label).toBe('Check: C5');
      expect(nodes.find((n) => n.id === 'S1')?.data.label).toBe('Stage: S1');
    });

    it('should create standalone nodes', () => {
      const graph: TurboCIGraphView = {
        checks: [createCheckView('C_Node')],
        stages: [createStageView('S_Node')],
      };

      const { nodes } = new TurboCIGraphBuilder(graph).build();
      const cNode = nodes.find((n) => n.id === 'C_Node');
      const sNode = nodes.find((n) => n.id === 'S_Node');

      expect(cNode).toBeDefined();
      expect(sNode).toBeDefined();

      expect(cNode?.data.isGrouped).toBe(false);
      expect(sNode?.data.isGrouped).toBe(false);
    });
  });

  describe('Edges', () => {
    it('should create edges from dependency to dependent (Source -> Target)', () => {
      const c1Ident = createCheckIdentifier('C1');
      // S1 depends on C1
      const graph: TurboCIGraphView = {
        checks: [createCheckView('C1')],
        stages: [createStageView('S1', [], [c1Ident])],
      };

      const { edges, nodes } = new TurboCIGraphBuilder(graph).build();

      expect(nodes).toHaveLength(2);
      expect(edges).toHaveLength(1);
      expect(edges[0].id).toBe('dep-C1-S1');
      expect(edges[0].source).toBe('C1'); // The dependency
      expect(edges[0].target).toBe('S1'); // The dependent
    });

    it('should handle stage-to-stage dependencies', () => {
      const s1Ident = createStageIdentifier('S1');
      // S2 depends on S1
      const graph: TurboCIGraphView = {
        checks: [],
        stages: [createStageView('S1'), createStageView('S2', [], [s1Ident])],
      };

      const { edges } = new TurboCIGraphBuilder(graph).build();

      expect(edges).toHaveLength(1);
      expect(edges[0].source).toBe('S1');
      expect(edges[0].target).toBe('S2');
    });

    it('should not create edges if the target identifier does not exist in the graph', () => {
      const ghostIdent = createCheckIdentifier('Ghost');
      const graph: TurboCIGraphView = {
        checks: [
          createCheckView('C1', CheckKind.CHECK_KIND_BUILD, [ghostIdent]),
        ],
        stages: [],
      };

      const { nodes, edges } = new TurboCIGraphBuilder(graph).build();
      expect(nodes).toHaveLength(1);
      expect(edges).toHaveLength(0);
    });
  });

  describe('Grouping and Layout', () => {
    it('should group a stage assigned to a check and suppress internal edges', () => {
      const c1Ident = createCheckIdentifier('C1');
      // S1 assigned to C1, and also declares a dependency on C1.
      const graph: TurboCIGraphView = {
        checks: [createCheckView('C1')],
        stages: [createStageView('S1', ['C1'], [c1Ident])],
      };

      const { nodes, edges } = new TurboCIGraphBuilder(graph).build();

      // 1. No visual edges created within a group, despite the dependency
      expect(edges).toHaveLength(0);

      const s1 = nodes.find((n) => n.id === 'S1')!;
      const c1 = nodes.find((n) => n.id === 'C1')!;

      // 2. Marked as grouped
      expect(s1.data.isGrouped).toBe(true);
      expect(c1.data.isGrouped).toBe(true);

      // 3. Layout: Vertically stacked (same X due to Dagre meta-node)
      expect(s1.position.x).toBe(c1.position.x);
      // Stage is placed above check in Y axis
      expect(s1.position.y).toBeLessThan(c1.position.y);
    });

    it('should stack multiple stages alphabetically on a check', () => {
      // IDs: Stage_Z, Stage_A assigned to C1.
      // Builder sorts stage IDs.
      // Expected Visual Stack (Top down): Stage_A -> Stage_Z -> C1
      const graph: TurboCIGraphView = {
        checks: [createCheckView('C1')],
        stages: [
          createStageView('Stage_Z', ['C1']),
          createStageView('Stage_A', ['C1']),
        ],
      };

      const { nodes } = new TurboCIGraphBuilder(graph).build();

      const sA = nodes.find((n) => n.id === 'Stage_A')!;
      const sZ = nodes.find((n) => n.id === 'Stage_Z')!;
      const c1 = nodes.find((n) => n.id === 'C1')!;

      // All aligned horizontally
      expect(sA.position.x).toBe(sZ.position.x);
      expect(sZ.position.x).toBe(c1.position.x);

      // Vertical order based on alphabetical ID sort (A above Z above C1)
      expect(sA.position.y).toBeLessThan(sZ.position.y);
      expect(sZ.position.y).toBeLessThan(c1.position.y);
    });

    it('should handle a stack of 3+ stages correctly (Top, Middle, Bottom styles)', () => {
      const graph: TurboCIGraphView = {
        checks: [createCheckView('C1')],
        stages: [
          createStageView('S_B', ['C1']), // Will be Middle
          createStageView('S_C', ['C1']), // Will be Bottom
          createStageView('S_A', ['C1']), // Will be Top
        ],
      };

      const { nodes } = new TurboCIGraphBuilder(graph).build();

      const middleNode = nodes.find((n) => n.id === 'S_B')!;

      // Verify positioning logic holds
      const top = nodes.find((n) => n.id === 'S_A')!;
      const bottom = nodes.find((n) => n.id === 'S_C')!;
      expect(top.position.y).toBeLessThan(middleNode.position.y);
      expect(middleNode.position.y).toBeLessThan(bottom.position.y);
    });

    it('should treat stages assigned to multiple checks as standalone', () => {
      // Builder logic filters out stages with assignments.length !== 1 from groups.
      const graph: TurboCIGraphView = {
        checks: [createCheckView('C1'), createCheckView('C2')],
        stages: [createStageView('S_Multi', ['C1', 'C2'])],
      };

      const { nodes } = new TurboCIGraphBuilder(graph).build();
      const sMulti = nodes.find((n) => n.id === 'S_Multi')!;
      const c1 = nodes.find((n) => n.id === 'C1')!;

      expect(sMulti.data.isGrouped).toBeFalsy();
      expect(c1.data.isGrouped).toBeFalsy();
    });

    it('should treat stages assigned to zero checks as standalone', () => {
      const graph: TurboCIGraphView = {
        checks: [],
        stages: [createStageView('S_None', [])],
      };

      const { nodes } = new TurboCIGraphBuilder(graph).build();
      const sNone = nodes.find((n) => n.id === 'S_None')!;
      expect(sNone.data.isGrouped).toBeFalsy();
    });
  });

  describe('Inter-Group Dependencies', () => {
    it('should draw edges between nodes in different groups', () => {
      const c1Ident = createCheckIdentifier('C1');
      // Group 1: [S1 assigned to C1]
      // Group 2: [S2 assigned to C2]
      // Dependency: C2 depends on C1.
      const graph: TurboCIGraphView = {
        checks: [
          createCheckView('C1'),
          createCheckView('C2', CheckKind.CHECK_KIND_TEST, [c1Ident]),
        ],
        stages: [createStageView('S1', ['C1']), createStageView('S2', ['C2'])],
      };

      const { edges, nodes } = new TurboCIGraphBuilder(graph).build();

      expect(nodes).toHaveLength(4);
      // Dagre lays out Group(C1) -> Group(C2).
      // ReactFlow edge connects actual nodes C1 -> C2.
      expect(edges).toHaveLength(1);
      expect(edges[0].source).toBe('C1');
      expect(edges[0].target).toBe('C2');
    });

    it('should draw edges from a stage in one group to a stage in another', () => {
      const s1Ident = createStageIdentifier('S1');
      // Group 1: [S1 -> C1]
      // Group 2: [S2 -> C2]
      // Dependency: S2 depends on S1.
      const graph: TurboCIGraphView = {
        checks: [createCheckView('C1'), createCheckView('C2')],
        stages: [
          createStageView('S1', ['C1']),
          createStageView('S2', ['C2'], [s1Ident]),
        ],
      };

      const { edges } = new TurboCIGraphBuilder(graph).build();

      expect(edges).toHaveLength(1);
      expect(edges[0].source).toBe('S1');
      expect(edges[0].target).toBe('S2');
    });

    it('should draw edges from a standalone node to a node inside a group', () => {
      const cStandaloneIdent = createCheckIdentifier('C_Standalone');
      // Standalone: C_Standalone
      // Group: [S1 assigned to C1]
      // Dependency: S1 depends on C_Standalone
      const graph: TurboCIGraphView = {
        checks: [createCheckView('C_Standalone'), createCheckView('C1')],
        stages: [createStageView('S1', ['C1'], [cStandaloneIdent])],
      };

      const { edges, nodes } = new TurboCIGraphBuilder(graph).build();

      // Dagre lays out C_Standalone -> Group(C1)
      const cStandalone = nodes.find((n) => n.id === 'C_Standalone')!;
      const s1 = nodes.find((n) => n.id === 'S1')!;

      // Source should be to the left of target based on 'rankdir: LR'
      expect(cStandalone.position.x).toBeLessThan(s1.position.x);

      expect(edges).toHaveLength(1);
      expect(edges[0].source).toBe('C_Standalone');
      expect(edges[0].target).toBe('S1');
    });
  });

  describe('Error Handling', () => {
    it('should throw if a stage lacks an identifier', () => {
      const graph = {
        checks: [],
        stages: [{ stage: {} }], // Invalid StageView: missing identifier
      } as unknown as TurboCIGraphView;

      expect(() => new TurboCIGraphBuilder(graph).build()).toThrow(
        /Invalid StageView/,
      );
    });

    it('should throw if a check lacks an identifier', () => {
      const graph = {
        checks: [{ check: { kind: CheckKind.CHECK_KIND_BUILD } }], // Missing Identifier
        stages: [],
      } as unknown as TurboCIGraphView;

      expect(() => new TurboCIGraphBuilder(graph).build()).toThrow(
        /Invalid CheckView/,
      );
    });

    it('should throw if identifier.id is missing', () => {
      const graph = {
        checks: [
          { check: { identifier: { workPlan: WORKPLAN } } }, // Missing ID string
        ],
        stages: [],
      } as unknown as TurboCIGraphView;

      expect(() => new TurboCIGraphBuilder(graph).build()).toThrow(
        /Invalid CheckView/,
      );
    });
  });
});

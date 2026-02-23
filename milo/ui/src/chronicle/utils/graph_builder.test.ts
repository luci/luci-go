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
  Stage as StageId,
  WorkPlan,
} from '@/proto/turboci/graph/ids/v1/identifier.pb';
import { Check } from '@/proto/turboci/graph/orchestrator/v1/check.pb';
import { CheckKind } from '@/proto/turboci/graph/orchestrator/v1/check_kind.pb';
import { Dependencies } from '@/proto/turboci/graph/orchestrator/v1/dependencies.pb';
import { Stage_Assignment } from '@/proto/turboci/graph/orchestrator/v1/stage.pb';
import { Stage } from '@/proto/turboci/graph/orchestrator/v1/stage.pb';
import { WorkPlan as TurboCIGraphWorkPlan } from '@/proto/turboci/graph/orchestrator/v1/workplan.pb';

import { TYPE_URL_BUILD_RESULT } from './check_utils';
import {
  GroupMode,
  TurboCIGraphBuilder,
  getTopologyGroups,
} from './graph_builder';

const WORKPLAN: WorkPlan = { id: 'test-plan' };

/** Creates a general Identifier pointing to a Check. */
function createCheckIdentifier(id: string): CheckId {
  return { workPlan: WORKPLAN, id };
}

/** Creates a general Identifier pointing to a Stage. */
function createStageIdentifier(id: string): StageId {
  return { workPlan: WORKPLAN, id };
}

/** Creates a partial CheckView for testing. */
function createCheck(
  id: string,
  kind: CheckKind = CheckKind.CHECK_KIND_BUILD,
  checkDependencies: CheckId[] = [],
  stageDependencies: StageId[] = [],
  isSuccess?: boolean,
): Check {
  const deps: Dependencies = {
    edges: [
      ...checkDependencies.map((c) => ({ check: { identifier: c } })),
      ...stageDependencies.map((s) => ({ stage: { identifier: s } })),
    ],
    resolutionEvents: {},
  };

  const checkId: CheckId = { workPlan: WORKPLAN, id };

  const results = [];
  if (isSuccess !== undefined) {
    results.push({
      data: [
        {
          value: {
            value: { typeUrl: TYPE_URL_BUILD_RESULT, value: new Uint8Array() },
            valueJson: JSON.stringify({ success: isSuccess }),
          },
        },
      ],
    });
  }

  return {
    identifier: checkId,
    kind: kind,
    dependencies: deps,
    options: [],
    results,
    stateHistory: [],
    edits: [],
    // other fields like realm/version/state can be undefined
  } as Check;
}

/** Creates a partial StageView for testing. */
function createStage(
  id: string,
  // IDs of checks this stage is assigned to.
  assignedToCheckIds: string[] = [],
  checkDependencies: CheckId[] = [],
  stageDependencies: StageId[] = [],
): Stage {
  const deps: Dependencies = {
    edges: [
      ...checkDependencies.map((c) => ({ check: { identifier: c } })),
      ...stageDependencies.map((s) => ({ stage: { identifier: s } })),
    ],
    resolutionEvents: {},
  };

  const assignments: Stage_Assignment[] = assignedToCheckIds.map(
    (checkIdStr) => ({
      target: { workPlan: WORKPLAN, id: checkIdStr },
    }),
  );

  const stageId: StageId = { workPlan: WORKPLAN, id };

  return {
    identifier: stageId,
    dependencies: deps,
    assignments: assignments,
    attempts: [],
    stateHistory: [],
    edits: [],
    // other fields can be undefined
  } as Stage;
}

describe('TurboCIGraphBuilder', () => {
  it('should return empty arrays for an empty graph', () => {
    const graph: TurboCIGraphWorkPlan = {
      id: '',
      checks: [],
      stages: [],
    } as unknown as TurboCIGraphWorkPlan;
    const builder = new TurboCIGraphBuilder(graph);
    const { nodes, edges } = builder.build();

    expect(nodes).toEqual([]);
    expect(edges).toEqual([]);
  });

  describe('Node Creation and Labeling', () => {
    it('should create nodes with correct labels based on CheckKind', () => {
      const graph: TurboCIGraphWorkPlan = {
        id: '',
        checks: [
          createCheck('C1', CheckKind.CHECK_KIND_SOURCE),
          createCheck('C2', CheckKind.CHECK_KIND_BUILD),
          createCheck('C3', CheckKind.CHECK_KIND_TEST),
          createCheck('C4', CheckKind.CHECK_KIND_ANALYSIS),
          createCheck('C5', CheckKind.CHECK_KIND_UNKNOWN),
        ],
        stages: [createStage('S1')],
      } as unknown as TurboCIGraphWorkPlan;

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
      const graph: TurboCIGraphWorkPlan = {
        id: '',
        checks: [createCheck('C_Node')],
        stages: [createStage('S_Node')],
      } as unknown as TurboCIGraphWorkPlan;

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
      const graph: TurboCIGraphWorkPlan = {
        id: '',
        checks: [createCheck('C1')],
        stages: [createStage('S1', [], [c1Ident])],
      } as unknown as TurboCIGraphWorkPlan;

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
      const graph: TurboCIGraphWorkPlan = {
        id: '',
        checks: [],
        stages: [createStage('S1'), createStage('S2', [], [s1Ident])],
      } as unknown as TurboCIGraphWorkPlan;

      const { edges } = new TurboCIGraphBuilder(graph).build();

      expect(edges).toHaveLength(1);
      expect(edges[0].source).toBe('S1');
      expect(edges[0].target).toBe('S2');
    });

    it('should not create edges if the target identifier does not exist in the graph', () => {
      const ghostIdent = createCheckIdentifier('Ghost');
      const graph: TurboCIGraphWorkPlan = {
        id: '',
        checks: [createCheck('C1', CheckKind.CHECK_KIND_BUILD, [ghostIdent])],
        stages: [],
      } as unknown as TurboCIGraphWorkPlan;

      const { nodes, edges } = new TurboCIGraphBuilder(graph).build();
      expect(nodes).toHaveLength(1);
      expect(edges).toHaveLength(0);
    });
  });

  describe('Grouping and Layout', () => {
    it('should group a stage assigned to a check and suppress internal edges', () => {
      const c1Ident = createCheckIdentifier('C1');
      // S1 assigned to C1, and also declares a dependency on C1.
      const graph: TurboCIGraphWorkPlan = {
        id: '',
        checks: [createCheck('C1')],
        stages: [createStage('S1', ['C1'], [c1Ident])],
      } as unknown as TurboCIGraphWorkPlan;

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
      const graph: TurboCIGraphWorkPlan = {
        id: '',
        checks: [createCheck('C1')],
        stages: [
          createStage('Stage_Z', ['C1']),
          createStage('Stage_A', ['C1']),
        ],
      } as unknown as TurboCIGraphWorkPlan;

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
      const graph: TurboCIGraphWorkPlan = {
        id: '',
        checks: [createCheck('C1')],
        stages: [
          createStage('S_B', ['C1']), // Will be Middle
          createStage('S_C', ['C1']), // Will be Bottom
          createStage('S_A', ['C1']), // Will be Top
        ],
      } as unknown as TurboCIGraphWorkPlan;

      const { nodes } = new TurboCIGraphBuilder(graph).build();

      const middleNode = nodes.find((n) => n.id === 'S_B')!;

      // Verify positioning logic holds
      const top = nodes.find((n) => n.id === 'S_A')!;
      const bottom = nodes.find((n) => n.id === 'S_C')!;
      expect(top.position.y).toBeLessThan(middleNode.position.y);
      expect(middleNode.position.y).toBeLessThan(bottom.position.y);
    });

    it('should treat stages assigned to multiple checks as standalone but create edges for them', () => {
      // Builder logic filters out stages with assignments.length !== 1 from groups.
      const graph: TurboCIGraphWorkPlan = {
        id: '',
        checks: [createCheck('C1'), createCheck('C2')],
        stages: [createStage('S_Multi', ['C1', 'C2'])],
      } as unknown as TurboCIGraphWorkPlan;

      const { nodes, edges } = new TurboCIGraphBuilder(graph).build({
        showAssignmentEdges: true,
      });
      const sMulti = nodes.find((n) => n.id === 'S_Multi')!;
      const c1 = nodes.find((n) => n.id === 'C1')!;

      expect(sMulti.data.isGrouped).toBeFalsy();
      expect(c1.data.isGrouped).toBeFalsy();

      expect(edges).toHaveLength(2);
      expect(edges[0].id).toBe('assignment-S_Multi-C1');
      expect(edges[0].data?.isAssignment).toBeTruthy();
      expect(edges[0].style?.strokeWidth).toBe(1);
      expect(edges[1].id).toBe('assignment-S_Multi-C2');
      expect(edges[1].data?.isAssignment).toBeTruthy();
      expect(edges[1].style?.strokeWidth).toBe(1);
    });

    it('draws invisible assignment edges when showAssignmentEdges is false', () => {
      // Builder logic filters out stages with assignments.length !== 1 from groups.
      const graph: TurboCIGraphWorkPlan = {
        id: '',
        checks: [createCheck('C1'), createCheck('C2')],
        stages: [createStage('S_Multi', ['C1', 'C2'])],
      } as unknown as TurboCIGraphWorkPlan;

      const { nodes, edges } = new TurboCIGraphBuilder(graph).build({
        showAssignmentEdges: false,
      });
      const sMulti = nodes.find((n) => n.id === 'S_Multi')!;
      const c1 = nodes.find((n) => n.id === 'C1')!;

      expect(sMulti.data.isGrouped).toBeFalsy();
      expect(c1.data.isGrouped).toBeFalsy();

      expect(edges).toHaveLength(2);
      expect(edges[0].id).toBe('assignment-S_Multi-C1');
      expect(edges[0].data?.isAssignment).toBeTruthy();
      expect(edges[0].style?.strokeWidth).toBe(0);
      expect(edges[1].id).toBe('assignment-S_Multi-C2');
      expect(edges[1].data?.isAssignment).toBeTruthy();
      expect(edges[1].style?.strokeWidth).toBe(0);
    });

    it('should treat stages assigned to zero checks as standalone', () => {
      const graph: TurboCIGraphWorkPlan = {
        id: '',
        checks: [],
        stages: [createStage('S_None')],
      } as unknown as TurboCIGraphWorkPlan;

      const { nodes } = new TurboCIGraphBuilder(graph).build();
      const sNone = nodes.find((n) => n.id === 'S_None')!;
      expect(sNone.data.isGrouped).toBeFalsy();
    });
  });

  describe('Inter-Group Dependencies', () => {
    it('should draw edges between nodes in different groups', () => {
      const b1Ident = createCheckIdentifier('B1');
      // Graph: B1 -> S1 -> T1
      // Expected node order in X axis: B1, S1, T1
      const graph: TurboCIGraphWorkPlan = {
        id: '',
        checks: [
          createCheck('B1'),
          createCheck('T1', CheckKind.CHECK_KIND_TEST), // T1 assigned to S1 below instead of explicitly modeling the dep on T1 side.
        ],
        stages: [createStage('S1', ['T1'], [b1Ident])],
      } as unknown as TurboCIGraphWorkPlan;

      const { edges, nodes } = new TurboCIGraphBuilder(graph).build();

      expect(nodes).toHaveLength(3);
      // Dagre lays out B1 -> S1 -> T1.
      // ReactFlow edge connects actual nodes B1 -> S1 and S1 -> T1.
      expect(edges).toHaveLength(1);
      expect(edges[0].source).toBe('B1');
      expect(edges[0].target).toBe('S1');

      const b1 = nodes.find((n) => n.id === 'B1')!;
      const s1 = nodes.find((n) => n.id === 'S1')!;
      const t1 = nodes.find((n) => n.id === 'T1')!;

      // Source should be to the left of target based on 'rankdir: LR'
      expect(b1.position.x).toBeLessThan(s1.position.x);
      expect(s1.position.x).toBe(t1.position.x); // Stacked
    });

    it('should draw edges from a stage in one group to a stage in another', () => {
      const s1Ident = createStageIdentifier('S1');
      // Group 1: [S1 -> C1]
      // Group 2: [S2 -> C2]
      // Dependency: S2 depends on S1.
      const graph: TurboCIGraphWorkPlan = {
        id: '',
        checks: [createCheck('C1'), createCheck('C2')],
        stages: [
          createStage('S1', ['C1']),
          createStage('S2', ['C2'], [], [s1Ident]),
        ],
      } as unknown as TurboCIGraphWorkPlan;

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
      const graph: TurboCIGraphWorkPlan = {
        id: '',
        checks: [createCheck('C_Standalone'), createCheck('C1')],
        stages: [createStage('S1', ['C1'], [cStandaloneIdent])],
      } as unknown as TurboCIGraphWorkPlan;

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

  describe('Node Collapsing', () => {
    it('should assign identical groupId to nodes with identical topology', () => {
      const b1Ident = createCheckIdentifier('B1');
      const graph: TurboCIGraphWorkPlan = {
        id: '',
        checks: [
          createCheck('B1', CheckKind.CHECK_KIND_BUILD),
          createCheck('T1', CheckKind.CHECK_KIND_TEST, [b1Ident], [], true),
          createCheck('T2', CheckKind.CHECK_KIND_TEST, [b1Ident], [], true),
        ],
        stages: [],
      } as unknown as TurboCIGraphWorkPlan;

      const { groupIdToChecks } = getTopologyGroups(graph);
      // We expect one group containing T1 and T2 (size 2)
      // And one group containing B1 (size 1)
      const groupT = Array.from(groupIdToChecks.values()).find(
        (checks) => checks.length === 2,
      );
      expect(groupT).toBeDefined();

      const ids = groupT!.map((c) => c.identifier!.id!).sort();
      expect(ids).toEqual(['T1', 'T2']);

      // Force expansion to verify groupId on nodes
      const groupId = Array.from(groupIdToChecks.keys()).find(
        (k) => groupIdToChecks.get(k) === groupT,
      )!;
      const groupModes = new Map([[groupId, GroupMode.EXPANDED]]);
      const { nodes } = new TurboCIGraphBuilder(graph).build({ groupModes });

      const t1 = nodes.find((n) => n.id === 'T1')!;
      const t2 = nodes.find((n) => n.id === 'T2')!;

      expect(t1.data.groupId).toBeDefined();
      expect(t1.data.groupId).toBe(groupId);
      expect(t2.data.groupId).toBe(groupId);
    });

    it('should assign identical groupId to nodes with identical topology regardless of status', () => {
      const b1Ident = createCheckIdentifier('B1');
      const graph: TurboCIGraphWorkPlan = {
        id: '',
        checks: [
          createCheck('B1', CheckKind.CHECK_KIND_BUILD),
          createCheck('T1', CheckKind.CHECK_KIND_TEST, [b1Ident], [], true), // Success
          createCheck('T2', CheckKind.CHECK_KIND_TEST, [b1Ident], [], false), // Failed
        ],
        stages: [],
      } as unknown as TurboCIGraphWorkPlan;

      const { groupIdToChecks } = getTopologyGroups(graph);
      // T1 and T2 should be in the same group because they share the same parent B1
      const groupT = Array.from(groupIdToChecks.values()).find(
        (checks) => checks.length === 2,
      );
      expect(groupT).toBeDefined();
      const ids = groupT!.map((c) => c.identifier!.id!).sort();
      expect(ids).toEqual(['T1', 'T2']);
    });

    it('should assign different groupIds for unique dependency sets', () => {
      const b1Ident = createCheckIdentifier('B1');
      const b2Ident = createCheckIdentifier('B2');

      const graph: TurboCIGraphWorkPlan = {
        id: '',
        checks: [
          createCheck('B1'),
          createCheck('B2'),
          createCheck('T1', CheckKind.CHECK_KIND_TEST, [b1Ident], [], true),
          createCheck('T2', CheckKind.CHECK_KIND_TEST, [b2Ident], [], true),
        ],
        stages: [],
      } as unknown as TurboCIGraphWorkPlan;

      // Ensure nodes are expanded to check IDs
      const { nodes } = new TurboCIGraphBuilder(graph).build();
      const t1 = nodes.find((n) => n.id === 'T1')!;
      const t2 = nodes.find((n) => n.id === 'T2')!;

      expect(t1.data.groupId).toBeDefined();
      expect(t2.data.groupId).toBeDefined();
      expect(t1.data.groupId).not.toBe(t2.data.groupId);
    });

    it('should collapse successful nodes by default (COLLAPSE_SUCCESS_ONLY)', () => {
      const b1Ident = createCheckIdentifier('B1');
      const graph: TurboCIGraphWorkPlan = {
        id: '',
        checks: [
          createCheck('B1'),
          createCheck('T1', CheckKind.CHECK_KIND_TEST, [b1Ident], [], true),
          createCheck('T2', CheckKind.CHECK_KIND_TEST, [b1Ident], [], true),
        ],
        stages: [],
      } as unknown as TurboCIGraphWorkPlan;

      // Default behavior is COLLAPSE_SUCCESS_ONLY
      const { nodes } = new TurboCIGraphBuilder(graph).build();

      // T1 and T2 should be gone
      expect(nodes.find((n) => n.id === 'T1')).toBeUndefined();
      expect(nodes.find((n) => n.id === 'T2')).toBeUndefined();

      // Find the group ID
      const { groupIdToChecks } = getTopologyGroups(graph);
      const groupId = Array.from(groupIdToChecks.keys()).find(
        (k) => groupIdToChecks.get(k)!.length === 2,
      )!;

      // Group node should exist
      const groupNodeId = `collapsed-group-${groupId}-success`;
      const groupNode = nodes.find((n) => n.id === groupNodeId);
      expect(groupNode).toBeDefined();
      expect(groupNode?.data.isCollapsed).toBe(true);
    });

    it('should generate correct labels for collapsed groups', () => {
      const b1Ident = createCheckIdentifier('B1');
      const b2Ident = createCheckIdentifier('B2');
      const graph: TurboCIGraphWorkPlan = {
        id: '',
        checks: [
          createCheck('B1'),
          createCheck('B2'),
          // Group 1: 2 Success Builds
          createCheck('SB1', CheckKind.CHECK_KIND_BUILD, [b1Ident], [], true),
          createCheck('SB2', CheckKind.CHECK_KIND_BUILD, [b1Ident], [], true),
          // Group 2: 2 Failed Tests
          createCheck('FT1', CheckKind.CHECK_KIND_TEST, [b2Ident], [], false),
          createCheck('FT2', CheckKind.CHECK_KIND_TEST, [b2Ident], [], false),
        ],
        stages: [],
      } as unknown as TurboCIGraphWorkPlan;

      const { groupIdToChecks } = getTopologyGroups(graph);
      let failGroupId: number;
      let successGroupId: number;

      groupIdToChecks.forEach((checks, id) => {
        if (checks.some((c) => c.identifier?.id === 'FT1')) failGroupId = id;
        if (checks.some((c) => c.identifier?.id === 'SB1')) successGroupId = id;
      });

      // We explicitly collapse the failed group to check the label for mixed/failed groups.
      // The successful group collapses by default.
      const groupModes = new Map([[failGroupId!, GroupMode.COLLAPSE_ALL]]);

      const { nodes } = new TurboCIGraphBuilder(graph).build({ groupModes });

      const successNode = nodes.find(
        (n) => n.id === `collapsed-group-${successGroupId}-success`,
      );
      const failNode = nodes.find(
        (n) => n.id === `collapsed-group-${failGroupId}`,
      );

      expect(successNode?.data.label).toBe('2 successful builds');
      expect(failNode?.data.label).toBe('2 failed tests');
    });

    it('should map parents to child group IDs', () => {
      const b1Ident = createCheckIdentifier('B1');
      const graph: TurboCIGraphWorkPlan = {
        id: '',
        checks: [
          createCheck('B1'),
          // Group 1: Successful T1, T2
          createCheck('T1', CheckKind.CHECK_KIND_TEST, [b1Ident], [], true),
          createCheck('T2', CheckKind.CHECK_KIND_TEST, [b1Ident], [], true),
        ],
        stages: [],
      } as unknown as TurboCIGraphWorkPlan;

      const { parentToGroupIds, groupIdToChecks } = getTopologyGroups(graph);
      expect(parentToGroupIds.has('B1')).toBe(true);

      const groupIds = parentToGroupIds.get('B1')!;
      expect(groupIds.length).toBe(1);

      const checks = groupIdToChecks.get(groupIds[0])!;
      expect(checks.length).toBe(2);
    });

    it('should redirect edges from collapsed nodes', () => {
      // S1 depends on T1. T1 is collapsed with T2.
      // S1 should depend on the Group Node.

      const b1Ident = createCheckIdentifier('B1');
      const t1Ident = createCheckIdentifier('T1');

      const graph: TurboCIGraphWorkPlan = {
        id: '',
        checks: [
          createCheck('B1'),
          createCheck('T1', CheckKind.CHECK_KIND_TEST, [b1Ident], [], true),
          createCheck('T2', CheckKind.CHECK_KIND_TEST, [b1Ident], [], true),
        ],
        stages: [createStage('S_Downstream', [], [t1Ident])],
      } as unknown as TurboCIGraphWorkPlan;

      const { groupIdToChecks } = getTopologyGroups(graph);
      const groupId = Array.from(groupIdToChecks.keys()).find(
        (k) => groupIdToChecks.get(k)!.length === 2,
      )!;

      // Default behavior collapses T1/T2 into a success group
      const { edges } = new TurboCIGraphBuilder(graph).build();

      const groupNodeId = `collapsed-group-${groupId}-success`;
      const edge = edges.find(
        (e) => e.source === groupNodeId && e.target === 'S_Downstream',
      );
      expect(edge).toBeDefined();
    });

    it('should NOT collapse nodes with identical parents but different children', () => {
      const b1Ident = createCheckIdentifier('B1');
      const t1Ident = createCheckIdentifier('T1');
      const t2Ident = createCheckIdentifier('T2');

      // T1 and T2 have the same parent (B1), but T1 -> C_Child1 and T2 -> C_Child2.
      // They should NOT collapse.
      const graph: TurboCIGraphWorkPlan = {
        id: '',
        checks: [
          createCheck('B1'),
          createCheck('T1', CheckKind.CHECK_KIND_TEST, [b1Ident], [], true),
          createCheck('T2', CheckKind.CHECK_KIND_TEST, [b1Ident], [], true),
          createCheck(
            'C_Child1',
            CheckKind.CHECK_KIND_ANALYSIS,
            [t1Ident],
            [],
            true,
          ),
          createCheck(
            'C_Child2',
            CheckKind.CHECK_KIND_ANALYSIS,
            [t2Ident],
            [],
            true,
          ),
        ],
        stages: [],
      } as unknown as TurboCIGraphWorkPlan;

      const { groupIdToChecks } = getTopologyGroups(graph);
      // T1 and T2 should NOT be in the same group because they have different children
      const groupT = Array.from(groupIdToChecks.values()).find(
        (checks) => checks.length === 2,
      );
      expect(groupT).toBeUndefined();
    });

    it('should collapse nodes with identical parents AND identical children', () => {
      const b1Ident = createCheckIdentifier('B1');
      const t1Ident = createCheckIdentifier('T1');
      const t2Ident = createCheckIdentifier('T2');

      // T1 and T2 have same parent (B1) and both feed into C_Common.
      // They SHOULD collapse.
      const graph: TurboCIGraphWorkPlan = {
        checks: [
          createCheck('B1'),
          createCheck('T1', CheckKind.CHECK_KIND_TEST, [b1Ident], [], true),
          createCheck('T2', CheckKind.CHECK_KIND_TEST, [b1Ident], [], true),
          createCheck('C_Common', CheckKind.CHECK_KIND_BUILD, [
            t1Ident,
            t2Ident,
          ]),
        ],
        stages: [],
      } as unknown as TurboCIGraphWorkPlan;

      const { groupIdToChecks } = getTopologyGroups(graph);
      const groupT = Array.from(groupIdToChecks.values()).find(
        (checks) => checks.length === 2,
      );
      expect(groupT).toBeDefined();
    });
  });
});

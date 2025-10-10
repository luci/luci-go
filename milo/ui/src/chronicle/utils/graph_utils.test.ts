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

import { CheckKind } from '@/proto/turboci/graph/orchestrator/v1/check_kind.pb';
import { GraphView as TurboCIGraphView } from '@/proto/turboci/graph/orchestrator/v1/graph_view.pb';

import {
  FakeGraphGenerator,
  GraphGenerationConfig,
} from '../fake_turboci_graph';

import { convertTurboCIGraphToNodesAndEdges } from './graph_utils';

describe('graph_utils', () => {
  describe('convertTurboCIGraphToNodesAndEdges', () => {
    it('should create nodes and edges correctly for a simple graph', () => {
      const config: GraphGenerationConfig = {
        workPlanIdStr: 'workplan-1',
        checkIds: ['C1', 'C2', 'C3'],
        stageIds: ['S1'],
        dependencies: {
          S1: ['C1', 'C2'],
          C3: ['C2'],
        },
      };
      const generator = new FakeGraphGenerator(config);
      const graph = generator.generate();

      const { nodes, edges } = convertTurboCIGraphToNodesAndEdges(graph);

      expect(nodes).toHaveLength(4);
      expect(edges).toHaveLength(3);

      // Check nodes
      const nodeIds = nodes.map((n) => n.id);
      expect(nodeIds).toEqual(['S1', 'C1', 'C2', 'C3']);

      // Check edges
      const edgeIds = edges.map((e) => e.id);
      expect(edgeIds).toEqual(['e-S1-C1', 'e-S1-C2', 'e-C3-C2']);

      const edge1 = edges.find((e) => e.id === 'e-S1-C1');
      expect(edge1?.source).toBe('C1');
      expect(edge1?.target).toBe('S1');

      const edge2 = edges.find((e) => e.id === 'e-S1-C2');
      expect(edge2?.source).toBe('C2');
      expect(edge2?.target).toBe('S1');

      const edge3 = edges.find((e) => e.id === 'e-C3-C2');
      expect(edge3?.source).toBe('C2');
      expect(edge3?.target).toBe('C3');
    });

    it('should have the right node labels for different check kinds', () => {
      const config: GraphGenerationConfig = {
        workPlanIdStr: 'workplan-1',
        checkIds: ['C1', 'C2', 'C3', 'C4'],
        stageIds: [],
        dependencies: {},
        checkKinds: {
          C1: CheckKind.CHECK_KIND_BUILD,
          C2: CheckKind.CHECK_KIND_TEST,
          C3: CheckKind.CHECK_KIND_SOURCE,
          C4: CheckKind.CHECK_KIND_ANALYSIS,
        },
      };
      const generator = new FakeGraphGenerator(config);
      const graph = generator.generate();

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

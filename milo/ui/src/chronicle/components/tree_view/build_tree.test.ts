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

import { Check } from '@/proto/turboci/graph/orchestrator/v1/check.pb';
import { CheckView } from '@/proto/turboci/graph/orchestrator/v1/check_view.pb';
import { Edge } from '@/proto/turboci/graph/orchestrator/v1/edge.pb';
import { Stage } from '@/proto/turboci/graph/orchestrator/v1/stage.pb';
import { StageView } from '@/proto/turboci/graph/orchestrator/v1/stage_view.pb';

import { buildVisualGraph, Graph, subtreeSize } from './build_tree';

const checkView = (check: Partial<Check>): CheckView => ({
  check: {
    identifier: { id: 'check1' },
    state: 40, // COMPLETE
    options: [],
    results: [],
    stateHistory: [],
    ...check,
  },
  optionData: {},
  edits: [],
  results: [],
});

const stageView = (stage: Partial<Stage>): StageView => ({
  stage: {
    identifier: { id: 'stage1' },
    state: 20, // RUNNING
    attempts: [],
    assignments: [],
    dependencies: {
      edges: [] as Edge[],
      resolutionEvents: {},
    },
    stateHistory: [],
    ...stage,
  },
  edits: [],
});

describe('BuildVisualGraph', () => {
  it('should return an empty graph when given no stages or checks', () => {
    const graph = buildVisualGraph([], []);
    expect(graph.nodes).toEqual({});
    expect(graph.roots).toEqual([]);
  });

  it('should handle a single check', () => {
    const checks: CheckView[] = [
      checkView({
        identifier: { id: 'check1' },
        state: 40, // COMPLETE
      }),
    ];
    const graph = buildVisualGraph([], checks);
    expect(Object.keys(graph.nodes)).toHaveLength(1);
    expect(graph.nodes['check1']).toBeDefined();
    expect(graph.nodes['check1'].status).toBe('FINAL');
    expect(graph.roots).toEqual(['check1']);
  });

  it('should handle a single stage with one check', () => {
    const stages: StageView[] = [
      stageView({
        identifier: { id: 'stage1' },
        state: 20, // RUNNING
        assignments: [{ target: { id: 'check1' } }],
      }),
    ];
    const checks: CheckView[] = [
      checkView({
        identifier: { id: 'check1' },
        state: 30, // WAITING
      }),
    ];
    const graph = buildVisualGraph(stages, checks);
    expect(Object.keys(graph.nodes)).toHaveLength(2);
    expect(graph.nodes['stage1']).toBeDefined();
    expect(graph.nodes['check1']).toBeDefined();
    expect(graph.nodes['stage1'].status).toBe('ATTEMPTING');
    expect(graph.nodes['check1'].status).toBe('WAITING');
    expect(graph.nodes['check1'].children).toContain('stage1');
    expect(graph.nodes['stage1'].parents).toContain('check1');
    expect(graph.roots).toEqual(['check1']);
  });

  it('should handle dependencies between checks', () => {
    const stages: StageView[] = [
      stageView({
        identifier: { id: 'stage1' },
        assignments: [{ target: { id: 'check2' } }],
        dependencies: {
          edges: [{ check: { identifier: { id: 'check1' } } }],
          resolutionEvents: {},
        },
      }),
    ];
    const checks: CheckView[] = [
      checkView({ identifier: { id: 'check1' }, state: 40 }),
      checkView({
        identifier: { id: 'check2' },
        state: 10,
      }),
    ];
    const graph = buildVisualGraph(stages, checks);
    expect(Object.keys(graph.nodes)).toHaveLength(3);
    expect(graph.nodes['check1'].children).toContain('check2');
    expect(graph.nodes['check2'].parents).toContain('check1');
    expect(graph.roots).toEqual(['check1']);
  });

  it('should handle stages with multiple assignments and dependencies', () => {
    const stages: StageView[] = [
      // Build stage with multiple assignments.
      stageView({
        identifier: { id: 'build-stage' },
        assignments: [
          { target: { id: 'source-check' } },
          { target: { id: 'build-check' } },
          { target: { id: 'test-check' } },
        ],
        // This stage depends on nothing.
        dependencies: { edges: [], resolutionEvents: {} },
      }),
      // E2E stage with multiple assignments, depending on the build-check.
      stageView({
        identifier: { id: 'e2e-stage' },
        assignments: [
          { target: { id: 'e2e-a-check' } },
          { target: { id: 'e2e-b-check' } },
        ],
        dependencies: {
          edges: [{ check: { identifier: { id: 'build-check' } } }],
          resolutionEvents: {},
        },
      }),
    ];
    const checks: CheckView[] = [
      checkView({ identifier: { id: 'source-check' } }),
      checkView({ identifier: { id: 'build-check' } }),
      checkView({ identifier: { id: 'test-check' } }),
      checkView({ identifier: { id: 'e2e-a-check' } }),
      checkView({ identifier: { id: 'e2e-b-check' } }),
    ];

    const graph = buildVisualGraph(stages, checks);

    // The e2e checks should be children of the build check.
    expect(graph.nodes['build-check'].children).toContain('e2e-a-check');
    expect(graph.nodes['build-check'].children).toContain('e2e-b-check');

    // The build check should be a parent of both e2e checks.
    expect(graph.nodes['e2e-a-check'].parents).toContain('build-check');
    expect(graph.nodes['e2e-b-check'].parents).toContain('build-check');

    // The root should be the source check, as it has no parents.
    // Note: build-check and test-check are also roots in this scenario
    // because they also have no parents. The order is alphabetical.
    expect(graph.roots).toEqual(['build-check', 'source-check', 'test-check']);
  });
});

describe('SubtreeSize', () => {
  it('should return 1 for a single node', () => {
    const graph: Graph = {
      nodes: {
        a: {
          id: 'a',
          children: new Set(),
          parents: new Set(),
          label: 'a',
          status: 'FINAL',
          type: 'CHECK' as const,
        },
      },
      roots: ['a'],
    };
    expect(subtreeSize(graph, 'a')).toBe(1);
  });

  it('should calculate the size of a simple subtree', () => {
    const graph: Graph = {
      nodes: {
        a: {
          id: 'a',
          children: new Set(['b']),
          parents: new Set(),
          label: 'a',
          status: 'FINAL',
          type: 'CHECK' as const,
        },
        b: {
          id: 'b',
          children: new Set(),
          parents: new Set(['a']),
          label: 'b',
          status: 'FINAL',
          type: 'CHECK' as const,
        },
      },
      roots: ['a'],
    };
    expect(subtreeSize(graph, 'a')).toBe(2);
  });

  it('should handle graphs with cycles', () => {
    const graph: Graph = {
      nodes: {
        a: {
          id: 'a',
          children: new Set(['b']),
          parents: new Set(['c']),
          label: 'a',
          status: 'FINAL',
          type: 'CHECK' as const,
        },
        b: {
          id: 'b',
          children: new Set(['c']),
          parents: new Set(['a']),
          label: 'b',
          status: 'FINAL',
          type: 'CHECK' as const,
        },
        c: {
          id: 'c',
          children: new Set(['a']),
          parents: new Set(['b']),
          label: 'c',
          status: 'FINAL',
          type: 'CHECK' as const,
        },
      },
      roots: ['a'],
    };
    expect(subtreeSize(graph, 'a')).toBe(3);
  });

  it('should handle multiple branches', () => {
    const graph: Graph = {
      nodes: {
        a: {
          id: 'a',
          children: new Set(['b', 'c']),
          parents: new Set(),
          label: 'a',
          status: 'FINAL',
          type: 'CHECK' as const,
        },
        b: {
          id: 'b',
          children: new Set(),
          parents: new Set(['a']),
          label: 'b',
          status: 'FINAL',
          type: 'CHECK' as const,
        },
        c: {
          id: 'c',
          children: new Set(['d']),
          parents: new Set(['a']),
          label: 'c',
          status: 'FINAL',
          type: 'CHECK' as const,
        },
        d: {
          id: 'd',
          children: new Set(),
          parents: new Set(['c']),
          label: 'd',
          status: 'FINAL',
          type: 'CHECK' as const,
        },
      },
      roots: ['a'],
    };
    expect(subtreeSize(graph, 'a')).toBe(4);
  });
});

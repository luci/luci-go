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

import { DateTime } from 'luxon';

import { Check } from '@/proto/turboci/graph/orchestrator/v1/check.pb';
import { CheckState } from '@/proto/turboci/graph/orchestrator/v1/check_state.pb';
import {
  Dependencies,
  Dependencies_Group,
} from '@/proto/turboci/graph/orchestrator/v1/dependencies.pb';
import {
  Edge,
  Resolution,
} from '@/proto/turboci/graph/orchestrator/v1/edge.pb';
import { Stage } from '@/proto/turboci/graph/orchestrator/v1/stage.pb';
import { StageState } from '@/proto/turboci/graph/orchestrator/v1/stage_state.pb';

import { computeCriticalPath } from './critical_path';

const ORIGIN = DateTime.fromISO('2026-09-15T00:00:00Z');

function ts(minute: number): string {
  return ORIGIN.plus({ minutes: minute }).toISO()!;
}

/** A dependency edge together with the resolution the node recorded for it. */
interface DepSpec {
  /** Depend on this stage. */
  readonly onStage?: string;
  /** Optional target state for a stage dependency. */
  readonly stageState?: StageState;
  /** Depend on this check. */
  readonly onCheck?: string;
  /** Optional target state for a check dependency. */
  readonly checkState?: CheckState;
  /** Minute at which the edge's condition was met. */
  readonly satisfiedAt?: number;
  /** Defaults to satisfied. */
  readonly resolution?: Resolution;
}

interface StageSpec {
  readonly id: string;
  /** Minute at which the stage reached its final state. */
  readonly end: number;
  readonly deps?: readonly DepSpec[];
  readonly predicate?: Dependencies_Group;
  readonly continuationDeps?: readonly DepSpec[];
  readonly awaitingGroupAt?: number;
  readonly createdByStage?: string;
  /**
   * When false, no resolution events are recorded, mimicking dependencies
   * that are still in the planning state.
   */
  readonly tracked?: boolean;
}

function buildDependencies(
  deps: readonly DepSpec[],
  tracked: boolean,
  predicate?: Dependencies_Group,
): Dependencies {
  const resolutionEvents: Record<
    number,
    { resolution: Resolution; conditionVersion?: { ts: string } }
  > = {};
  const edges: Edge[] = deps.map((dep, index) => {
    if (tracked) {
      resolutionEvents[index] = {
        resolution: dep.resolution ?? Resolution.RESOLUTION_SATISFIED,
        conditionVersion:
          dep.satisfiedAt !== undefined
            ? { ts: ts(dep.satisfiedAt) }
            : undefined,
      };
    }
    if (dep.onCheck) {
      return Edge.fromPartial({
        check: {
          identifier: { id: dep.onCheck },
          condition: dep.checkState ? { onState: dep.checkState } : undefined,
        },
      });
    }
    return Edge.fromPartial({
      stage: {
        identifier: { id: dep.onStage },
        condition: dep.stageState ? { onState: dep.stageState } : undefined,
      },
    });
  });

  return Dependencies.fromPartial({
    edges,
    predicate,
    resolutionEvents,
  });
}

function makeStage(spec: StageSpec): Stage {
  const deps = spec.deps ?? [];
  const stateHistory = [
    {
      state: StageState.STAGE_STATE_ATTEMPTING,
      version: { ts: ORIGIN.toISO()! },
    },
    ...(spec.awaitingGroupAt !== undefined
      ? [
          {
            state: StageState.STAGE_STATE_AWAITING_GROUP,
            version: { ts: ts(spec.awaitingGroupAt) },
          },
        ]
      : []),
    {
      state: StageState.STAGE_STATE_FINAL,
      version: { ts: ts(spec.end) },
    },
  ];

  return Stage.fromPartial({
    identifier: { id: spec.id },
    state: StageState.STAGE_STATE_FINAL,
    createdBy: spec.createdByStage
      ? { stageAttempt: { stage: { id: spec.createdByStage } } }
      : undefined,
    dependencies: buildDependencies(deps, spec.tracked ?? true, spec.predicate),
    continuationGroup: spec.continuationDeps
      ? buildDependencies(spec.continuationDeps, spec.tracked ?? true)
      : undefined,
    stateHistory,
  });
}

function criticalPath(
  specs: readonly StageSpec[],
  checks: readonly Check[] = [],
): string[] {
  return [...computeCriticalPath(specs.map(makeStage), checks).stageIds].sort();
}

describe('computeCriticalPath', () => {
  it('returns empty for an empty timeline', () => {
    const result = computeCriticalPath([]);
    expect(result.path).toEqual([]);
    expect(result.stageIds.size).toBe(0);
    expect(result.checkIds.size).toBe(0);
  });

  it('returns the only stage when there are no dependencies', () => {
    expect(criticalPath([{ id: 'S1', end: 10 }])).toEqual(['S1']);
  });

  it('seeds the walk at the last stage to finish', () => {
    expect(
      criticalPath([
        { id: 'S1', end: 30 },
        { id: 'S2', end: 40 },
      ]),
    ).toEqual(['S2']);
  });

  it('follows a direct stage to stage edge', () => {
    expect(
      criticalPath([
        { id: 'S1', end: 10 },
        { id: 'S2', end: 20, deps: [{ onStage: 'S1', satisfiedAt: 10 }] },
      ]),
    ).toEqual(['S1', 'S2']);
  });

  it('walks a multi hop chain', () => {
    expect(
      criticalPath([
        { id: 'S1', end: 10 },
        {
          id: 'S2',
          end: 20,
          deps: [{ onStage: 'S1', satisfiedAt: 10 }],
        },
        { id: 'S3', end: 30, deps: [{ onStage: 'S2', satisfiedAt: 20 }] },
      ]),
    ).toEqual(['S1', 'S2', 'S3']);
  });

  it('follows the dependency that was satisfied last', () => {
    expect(
      criticalPath([
        { id: 'Slow', end: 10 },
        { id: 'Fast', end: 12 },
        {
          id: 'Sink',
          end: 20,
          deps: [
            { onStage: 'Fast', satisfiedAt: 5 },
            { onStage: 'Slow', satisfiedAt: 9 },
          ],
        },
      ]),
    ).toEqual(['Sink', 'Slow']);
  });

  it('ignores edges that were not satisfied in an OR group', () => {
    expect(
      criticalPath([
        { id: 'Taken', end: 10 },
        { id: 'Skipped', end: 18 },
        {
          id: 'Sink',
          end: 20,
          deps: [
            { onStage: 'Taken', satisfiedAt: 9 },
            {
              onStage: 'Skipped',
              satisfiedAt: 17,
              resolution: Resolution.RESOLUTION_UNSATISFIED,
            },
          ],
          predicate: {
            edges: [0, 1],
            groups: [],
            threshold: 1,
          },
        },
      ]),
    ).toEqual(['Sink', 'Taken']);
  });

  it('does not satisfy an AND group when one of its edges is unsatisfied', () => {
    expect(
      criticalPath([
        { id: 'Satisfied', end: 10 },
        { id: 'Unsatisfied', end: 18 },
        {
          id: 'Sink',
          end: 20,
          deps: [
            { onStage: 'Satisfied', satisfiedAt: 9 },
            {
              onStage: 'Unsatisfied',
              satisfiedAt: 17,
              resolution: Resolution.RESOLUTION_UNSATISFIED,
            },
          ],
        },
      ]),
    ).toEqual(['Sink']);
  });

  it('prefers the condition time over the stage end time', () => {
    expect(
      criticalPath([
        { id: 'Early', end: 8 },
        { id: 'Late', end: 15 },
        {
          id: 'Sink',
          end: 20,
          deps: [
            { onStage: 'Late', satisfiedAt: 2 },
            { onStage: 'Early', satisfiedAt: 7 },
          ],
        },
      ]),
    ).toEqual(['Early', 'Sink']);
  });

  it('falls back to the latest ending predecessor when untracked', () => {
    expect(
      criticalPath([
        { id: 'S1', end: 5 },
        { id: 'S2', end: 14 },
        {
          id: 'Sink',
          end: 20,
          tracked: false,
          deps: [{ onStage: 'S1' }, { onStage: 'S2' }],
        },
      ]),
    ).toEqual(['S2', 'Sink']);
  });

  it('skips edges pointing outside the timeline', () => {
    expect(
      criticalPath([
        { id: 'Sink', end: 20, deps: [{ onStage: 'Missing', satisfiedAt: 9 }] },
      ]),
    ).toEqual(['Sink']);
  });

  it('ignores a self referential edge', () => {
    expect(
      criticalPath([
        {
          id: 'Sink',
          end: 20,
          deps: [{ onStage: 'Sink', satisfiedAt: 9 }],
        },
      ]),
    ).toEqual(['Sink']);
  });

  it('terminates on a dependency cycle', () => {
    expect(
      criticalPath([
        {
          id: 'S1',
          end: 10,
          deps: [{ onStage: 'S2', satisfiedAt: 5 }],
        },
        {
          id: 'S2',
          end: 20,
          deps: [{ onStage: 'S1', satisfiedAt: 5 }],
        },
      ]),
    ).toEqual(['S1', 'S2']);
  });

  it('excludes parallel work that is not on the path', () => {
    expect(
      criticalPath([
        { id: 'Root', end: 5 },
        {
          id: 'Branch',
          end: 9,
          deps: [{ onStage: 'Root', satisfiedAt: 5 }],
        },
        {
          id: 'Sink',
          end: 20,
          deps: [{ onStage: 'Root', satisfiedAt: 5 }],
        },
      ]),
    ).toEqual(['Root', 'Sink']);
  });

  it('traverses checks, check edits, check finalizations, and repeated check states', () => {
    // S6 depends on C5 (FINAL) -> finalized by S5 -> S5 depends on C5 (WAITING)
    // -> C5 has no dependencies at WAITING so it steps back to PLANNED -> edited
    // by S3 (at min 25) and S4 (at min 35, slower) -> S4 depends on S2 -> S2
    // depends on C1 (FINAL) -> finalized by S1.
    const stages = [
      makeStage({ id: 'S1', end: 10 }),
      makeStage({
        id: 'S2',
        end: 20,
        deps: [
          {
            onCheck: 'C1',
            checkState: CheckState.CHECK_STATE_FINAL,
            satisfiedAt: 10,
          },
        ],
      }),
      makeStage({ id: 'S3', end: 25 }),
      makeStage({
        id: 'S4',
        end: 35,
        deps: [{ onStage: 'S2', satisfiedAt: 20 }],
      }),
      makeStage({
        id: 'S5',
        end: 50,
        deps: [
          {
            onCheck: 'C5',
            checkState: CheckState.CHECK_STATE_WAITING,
            satisfiedAt: 35,
          },
        ],
      }),
      makeStage({
        id: 'S6',
        end: 60,
        deps: [
          {
            onCheck: 'C5',
            checkState: CheckState.CHECK_STATE_FINAL,
            satisfiedAt: 50,
          },
        ],
      }),
      makeStage({ id: 'S_unrelated', end: 45 }),
    ];

    const checks = [
      Check.fromPartial({
        identifier: { id: 'C1' },
        state: CheckState.CHECK_STATE_FINAL,
        stateHistory: [
          { state: CheckState.CHECK_STATE_FINAL, version: { ts: ts(10) } },
        ],
        edits: [
          {
            version: { ts: ts(10) },
            createdBy: { stageAttempt: { stage: { id: 'S1' } } },
            check: { state: CheckState.CHECK_STATE_FINAL },
          },
        ],
      }),
      Check.fromPartial({
        identifier: { id: 'C5' },
        state: CheckState.CHECK_STATE_FINAL,
        stateHistory: [
          { state: CheckState.CHECK_STATE_PLANNED, version: { ts: ts(35) } },
          { state: CheckState.CHECK_STATE_WAITING, version: { ts: ts(35) } },
          { state: CheckState.CHECK_STATE_FINAL, version: { ts: ts(50) } },
        ],
        edits: [
          {
            version: { ts: ts(25) },
            createdBy: { stageAttempt: { stage: { id: 'S3' } } },
            check: { state: CheckState.CHECK_STATE_PLANNED },
          },
          {
            version: { ts: ts(35) },
            createdBy: { stageAttempt: { stage: { id: 'S4' } } },
            check: { state: CheckState.CHECK_STATE_PLANNED },
          },
          {
            version: { ts: ts(50) },
            createdBy: { stageAttempt: { stage: { id: 'S5' } } },
            check: { state: CheckState.CHECK_STATE_FINAL },
          },
        ],
      }),
      Check.fromPartial({
        identifier: { id: 'C_unrelated' },
        state: CheckState.CHECK_STATE_FINAL,
        stateHistory: [
          { state: CheckState.CHECK_STATE_FINAL, version: { ts: ts(40) } },
        ],
      }),
    ];

    const result = computeCriticalPath(stages, checks);
    expect([...result.stageIds].sort()).toEqual(['S1', 'S2', 'S4', 'S5', 'S6']);
    expect([...result.checkIds].sort()).toEqual(['C1', 'C5']);
    expect(
      result.path.map((step) => ({
        id: step.stage?.identifier?.id ?? step.check?.identifier?.id,
        state: step.state,
      })),
    ).toEqual([
      { id: 'S1', state: StageState.STAGE_STATE_FINAL },
      { id: 'C1', state: CheckState.CHECK_STATE_FINAL },
      { id: 'S2', state: StageState.STAGE_STATE_FINAL },
      { id: 'S4', state: StageState.STAGE_STATE_FINAL },
      { id: 'C5', state: CheckState.CHECK_STATE_WAITING },
      { id: 'S5', state: StageState.STAGE_STATE_FINAL },
      { id: 'C5', state: CheckState.CHECK_STATE_FINAL },
      { id: 'S6', state: StageState.STAGE_STATE_FINAL },
    ]);
  });

  it('steps back across check states when WAITING has no dependencies and resolves createdBy', () => {
    const stages = [
      makeStage({ id: 'S_root', end: 5 }),
      makeStage({
        id: 'S_plan',
        end: 15,
        createdByStage: 'S_root',
      }),
      makeStage({
        id: 'S_exec',
        end: 30,
        deps: [
          {
            onCheck: 'C1',
            checkState: CheckState.CHECK_STATE_WAITING,
            satisfiedAt: 15,
          },
        ],
      }),
      makeStage({
        id: 'S_sink',
        end: 40,
        deps: [
          {
            onCheck: 'C1',
            checkState: CheckState.CHECK_STATE_FINAL,
            satisfiedAt: 30,
          },
        ],
      }),
    ];

    const checks = [
      Check.fromPartial({
        identifier: { id: 'C1' },
        state: CheckState.CHECK_STATE_FINAL,
        edits: [
          {
            version: { ts: ts(15) },
            createdBy: { stageAttempt: { stage: { id: 'S_plan' } } },
            check: { state: CheckState.CHECK_STATE_PLANNED },
          },
          {
            version: { ts: ts(30) },
            createdBy: { stageAttempt: { stage: { id: 'S_exec' } } },
            check: { state: CheckState.CHECK_STATE_FINAL },
          },
        ],
      }),
    ];

    const result = computeCriticalPath(stages, checks);
    // S_sink -> C1(FINAL) -> S_exec -> C1(WAITING -> PLANNED) -> S_plan -> S_root.
    expect([...result.stageIds].sort()).toEqual([
      'S_exec',
      'S_plan',
      'S_root',
      'S_sink',
    ]);
    expect([...result.checkIds].sort()).toEqual(['C1']);
  });

  it('resolves a check created directly in CHECK_STATE_FINAL', () => {
    const stages = [
      makeStage({ id: 'S_creator', end: 15 }),
      makeStage({
        id: 'S_consumer',
        end: 30,
        deps: [
          {
            onCheck: 'C_direct_final',
            checkState: CheckState.CHECK_STATE_FINAL,
            satisfiedAt: 15,
          },
        ],
      }),
    ];

    const checks = [
      Check.fromPartial({
        identifier: { id: 'C_direct_final' },
        state: CheckState.CHECK_STATE_FINAL,
        createdBy: { stageAttempt: { stage: { id: 'S_creator' } } },
        stateHistory: [
          { state: CheckState.CHECK_STATE_FINAL, version: { ts: ts(15) } },
        ],
      }),
    ];

    const result = computeCriticalPath(stages, checks);
    expect([...result.stageIds].sort()).toEqual(['S_consumer', 'S_creator']);
    expect([...result.checkIds].sort()).toEqual(['C_direct_final']);
  });

  it('resolves check.dependencies when at CHECK_STATE_WAITING', () => {
    const stages = [
      makeStage({ id: 'S_dep_finalizer', end: 25 }),
      makeStage({ id: 'S_planner', end: 12 }),
      makeStage({
        id: 'S_consumer',
        end: 40,
        deps: [
          {
            onCheck: 'C2',
            checkState: CheckState.CHECK_STATE_WAITING,
            satisfiedAt: 25,
          },
        ],
      }),
    ];

    const checks = [
      Check.fromPartial({
        identifier: { id: 'C1' },
        state: CheckState.CHECK_STATE_FINAL,
        edits: [
          {
            version: { ts: ts(25) },
            createdBy: { stageAttempt: { stage: { id: 'S_dep_finalizer' } } },
            check: { state: CheckState.CHECK_STATE_FINAL },
          },
        ],
      }),
      Check.fromPartial({
        identifier: { id: 'C2' },
        state: CheckState.CHECK_STATE_WAITING,
        dependencies: buildDependencies(
          [
            {
              onCheck: 'C1',
              checkState: CheckState.CHECK_STATE_FINAL,
              satisfiedAt: 25,
            },
          ],
          true,
        ),
        edits: [
          {
            version: { ts: ts(12) },
            createdBy: { stageAttempt: { stage: { id: 'S_planner' } } },
            check: { state: CheckState.CHECK_STATE_PLANNED },
          },
        ],
      }),
    ];

    const result = computeCriticalPath(stages, checks);
    expect([...result.stageIds].sort()).toEqual([
      'S_consumer',
      'S_dep_finalizer',
    ]);
    expect([...result.checkIds].sort()).toEqual(['C1', 'C2']);
  });

  it('selects the earliest satisfied edge for an OR dependency group (threshold=1)', () => {
    expect(
      criticalPath([
        { id: 'Fast', end: 8 },
        { id: 'Slow', end: 18 },
        {
          id: 'Sink',
          end: 25,
          deps: [
            { onStage: 'Fast', satisfiedAt: 8 },
            { onStage: 'Slow', satisfiedAt: 18 },
          ],
          predicate: {
            edges: [0, 1],
            groups: [],
            threshold: 1,
          },
        },
      ]),
    ).toEqual(['Fast', 'Sink']);
  });

  it('selects the k-th earliest satisfied child for a threshold=k group', () => {
    expect(
      criticalPath([
        { id: 'S1', end: 5 },
        { id: 'S2', end: 12 },
        { id: 'S3', end: 19 },
        {
          id: 'Sink',
          end: 25,
          deps: [
            { onStage: 'S1', satisfiedAt: 5 },
            { onStage: 'S2', satisfiedAt: 12 },
            { onStage: 'S3', satisfiedAt: 19 },
          ],
          predicate: {
            edges: [0, 1, 2],
            groups: [],
            threshold: 2,
          },
        },
      ]),
    ).toEqual(['S2', 'Sink']);
  });

  it('evaluates nested branching predicate groups ((S1 && S2) || (S3 && S4))', () => {
    // Subgroup A (S1 @ 5, S2 @ 15) unblocks at 15 (blocker S2).
    // Subgroup B (S3 @ 10, S4 @ 12) unblocks at 12 (blocker S4).
    // Outer OR group (threshold=1) is unblocked by Subgroup B at 12 -> S4.
    expect(
      criticalPath([
        { id: 'S1', end: 5 },
        { id: 'S2', end: 15 },
        { id: 'S3', end: 10 },
        { id: 'S4', end: 12 },
        {
          id: 'Sink',
          end: 25,
          deps: [
            { onStage: 'S1', satisfiedAt: 5 },
            { onStage: 'S2', satisfiedAt: 15 },
            { onStage: 'S3', satisfiedAt: 10 },
            { onStage: 'S4', satisfiedAt: 12 },
          ],
          predicate: {
            edges: [],
            groups: [
              { edges: [0, 1], groups: [] },
              { edges: [2, 3], groups: [] },
            ],
            threshold: 1,
          },
        },
      ]),
    ).toEqual(['S4', 'Sink']);
  });

  it('ignores a partially satisfied nested AND subgroup inside an OR group', () => {
    // Subgroup A (S1 @ 2 satisfied, S2 unsatisfied) is an AND group, so it is
    // not satisfied even though S1 resolved earliest.
    // Subgroup B (S3 @ 10, S4 @ 12) is satisfied at 12 -> blocker S4.
    expect(
      criticalPath([
        { id: 'S1', end: 2 },
        { id: 'S2', end: 5 },
        { id: 'S3', end: 10 },
        { id: 'S4', end: 12 },
        {
          id: 'Sink',
          end: 25,
          deps: [
            { onStage: 'S1', satisfiedAt: 2 },
            {
              onStage: 'S2',
              satisfiedAt: 5,
              resolution: Resolution.RESOLUTION_UNSATISFIED,
            },
            { onStage: 'S3', satisfiedAt: 10 },
            { onStage: 'S4', satisfiedAt: 12 },
          ],
          predicate: {
            edges: [],
            groups: [
              { edges: [0, 1], groups: [] },
              { edges: [2, 3], groups: [] },
            ],
            threshold: 1,
          },
        },
      ]),
    ).toEqual(['S4', 'Sink']);
  });

  it('follows continuationGroup when it blocks STAGE_STATE_FINAL and steps back through creator ATTEMPTING state', () => {
    // ParentStage (FINAL) -> ChildStage (FINAL, via continuationGroup)
    // -> ParentStage (ATTEMPTING, via createdBy) -> InitDep (FINAL, via deps).
    expect(
      criticalPath([
        { id: 'InitDep', end: 5 },
        { id: 'FastChild', end: 18, createdByStage: 'ParentStage' },
        { id: 'ChildStage', end: 22, createdByStage: 'ParentStage' },
        {
          id: 'ParentStage',
          end: 25,
          awaitingGroupAt: 15,
          deps: [{ onStage: 'InitDep', satisfiedAt: 5 }],
          continuationDeps: [
            { onStage: 'FastChild', satisfiedAt: 18 },
            { onStage: 'ChildStage', satisfiedAt: 22 },
          ],
        },
      ]),
    ).toEqual(['ChildStage', 'InitDep', 'ParentStage']);
  });

  it('falls through to stage.dependencies when continuationGroup finished before AWAITING_GROUP', () => {
    expect(
      criticalPath([
        { id: 'InitDep', end: 5 },
        { id: 'EarlyChild', end: 10, createdByStage: 'ParentStage' },
        {
          id: 'ParentStage',
          end: 25,
          awaitingGroupAt: 15,
          deps: [{ onStage: 'InitDep', satisfiedAt: 5 }],
          continuationDeps: [{ onStage: 'EarlyChild', satisfiedAt: 10 }],
        },
      ]),
    ).toEqual(['InitDep', 'ParentStage']);
  });
});

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

import { Dependencies } from '@/proto/turboci/graph/orchestrator/v1/dependencies.pb';
import { Resolution } from '@/proto/turboci/graph/orchestrator/v1/edge.pb';
import { Stage } from '@/proto/turboci/graph/orchestrator/v1/stage.pb';
import { StageState } from '@/proto/turboci/graph/orchestrator/v1/stage_state.pb';

import { computeCriticalPath } from './critical_path';

const ORIGIN = DateTime.fromISO('2026-09-15T00:00:00Z');

function ts(minute: number): string {
  return ORIGIN.plus({ minutes: minute }).toISO()!;
}

/** A dependency edge together with the resolution the stage recorded for it. */
interface DepSpec {
  /** Depend directly on this stage. */
  readonly onStage: string;
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
  /**
   * When false, no resolution events are recorded, mimicking dependencies
   * that are still in the planning state.
   */
  readonly tracked?: boolean;
}

function buildDependencies(
  deps: readonly DepSpec[],
  tracked: boolean,
): Dependencies {
  const resolutionEvents: Record<
    number,
    { resolution: Resolution; conditionVersion?: { ts: string } }
  > = {};
  const edges = deps.map((dep, index) => {
    if (tracked) {
      resolutionEvents[index] = {
        resolution: dep.resolution ?? Resolution.RESOLUTION_SATISFIED,
        conditionVersion:
          dep.satisfiedAt !== undefined
            ? { ts: ts(dep.satisfiedAt) }
            : undefined,
      };
    }
    return { stage: { identifier: { id: dep.onStage } } };
  });

  return Dependencies.fromPartial({ edges, resolutionEvents });
}

function makeStage(spec: StageSpec): Stage {
  const deps = spec.deps ?? [];
  return Stage.fromPartial({
    identifier: { id: spec.id },
    dependencies: buildDependencies(deps, spec.tracked ?? true),
    stateHistory: [
      {
        state: StageState.STAGE_STATE_ATTEMPTING,
        version: { ts: ORIGIN.toISO()! },
      },
      {
        state: StageState.STAGE_STATE_FINAL,
        version: { ts: ts(spec.end) },
      },
    ],
  });
}

function criticalPath(specs: readonly StageSpec[]): string[] {
  return [...computeCriticalPath(specs.map(makeStage))].sort();
}

describe('computeCriticalPath', () => {
  it('returns empty for an empty timeline', () => {
    expect(computeCriticalPath([]).size).toBe(0);
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

  it('ignores edges that were not satisfied', () => {
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
        },
      ]),
    ).toEqual(['Sink', 'Taken']);
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
});

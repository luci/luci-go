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

import { Stage } from '@/proto/turboci/graph/orchestrator/v1/stage.pb';
import { StageAttemptState } from '@/proto/turboci/graph/orchestrator/v1/stage_attempt_state.pb';
import { ValueData } from '@/proto/turboci/graph/orchestrator/v1/value_data.pb';

import { buildProgressSegments } from './segments';

const STAGE_START = DateTime.fromISO('2026-09-15T00:00:00Z');
const STAGE_END = DateTime.fromISO('2026-09-15T01:00:00Z');

const EMPTY_VALUES = new Map<string, ValueData>();

function ts(minute: number): string {
  return STAGE_START.plus({ minutes: minute }).toISO()!;
}

function stageWithProgress(
  messages: Array<{ minute: number; message: string }>,
  attemptEndMinute?: number,
): Stage {
  return Stage.fromPartial({
    identifier: { id: 'S1' },
    attempts: [
      {
        state: StageAttemptState.STAGE_ATTEMPT_STATE_COMPLETE,
        stateHistory: [
          {
            state: StageAttemptState.STAGE_ATTEMPT_STATE_RUNNING,
            version: { ts: ts(0) },
          },
          ...(attemptEndMinute !== undefined
            ? [
                {
                  state: StageAttemptState.STAGE_ATTEMPT_STATE_COMPLETE,
                  version: { ts: ts(attemptEndMinute) },
                },
              ]
            : []),
        ],
        progress: messages.map((m) => ({
          message: m.message,
          version: { ts: ts(m.minute) },
        })),
      },
    ],
  });
}

describe('buildProgressSegments', () => {
  it('returns empty when a stage has no attempts', () => {
    const stage = Stage.fromPartial({
      identifier: { id: 'S1' },
      attempts: [],
    });
    expect(buildProgressSegments(stage, EMPTY_VALUES, STAGE_END)).toEqual([]);
  });

  it('returns empty when the attempt has no progress messages', () => {
    const stage = Stage.fromPartial({
      identifier: { id: 'S1' },
      attempts: [
        {
          stateHistory: [
            {
              state: StageAttemptState.STAGE_ATTEMPT_STATE_RUNNING,
              version: { ts: ts(0) },
            },
          ],
        },
      ],
    });
    expect(buildProgressSegments(stage, EMPTY_VALUES, STAGE_END)).toEqual([]);
  });

  it('starts the first progress segment at the attempt start time', () => {
    const stage = stageWithProgress(
      [{ minute: 5, message: 'first phase' }],
      30,
    );
    const segments = buildProgressSegments(stage, EMPTY_VALUES, STAGE_END);
    expect(segments).toHaveLength(1);
    expect(segments[0].label).toEqual('first phase');
    expect(segments[0].start.toISO()).toEqual(DateTime.fromISO(ts(0)).toISO());
    expect(segments[0].end.toISO()).toEqual(DateTime.fromISO(ts(30)).toISO());
  });

  it('starts each segment at its own message and ends it at the next', () => {
    const stage = stageWithProgress(
      [
        { minute: 10, message: 'sync' },
        { minute: 20, message: 'build' },
      ],
      30,
    );
    const segments = buildProgressSegments(stage, EMPTY_VALUES, STAGE_END);
    const byLabel = Object.fromEntries(segments.map((s) => [s.label, s]));

    expect(byLabel['sync'].start.toISO()).toEqual(
      DateTime.fromISO(ts(0)).toISO(),
    );
    expect(byLabel['sync'].end.toISO()).toEqual(
      DateTime.fromISO(ts(20)).toISO(),
    );
    expect(byLabel['build'].start.toISO()).toEqual(
      DateTime.fromISO(ts(20)).toISO(),
    );
    expect(byLabel['build'].end.toISO()).toEqual(
      DateTime.fromISO(ts(30)).toISO(),
    );
  });

  it('collapses consecutive segments with the same label', () => {
    const stage = stageWithProgress(
      [
        { minute: 5, message: 'sync' },
        { minute: 10, message: 'sync' },
        { minute: 15, message: 'build' },
      ],
      30,
    );
    const segments = buildProgressSegments(stage, EMPTY_VALUES, STAGE_END);
    expect(segments.map((s) => s.label)).toEqual(['sync', 'build']);
    const sync = segments.find((s) => s.label === 'sync')!;
    expect(sync.start.toISO()).toEqual(DateTime.fromISO(ts(0)).toISO());
    expect(sync.end.toISO()).toEqual(DateTime.fromISO(ts(15)).toISO());
  });

  it('does NOT collapse recurring labels if another label appeared in between', () => {
    const stage = stageWithProgress(
      [
        { minute: 5, message: 'sync' },
        { minute: 10, message: 'build' },
        { minute: 15, message: 'sync' },
      ],
      30,
    );
    const segments = buildProgressSegments(stage, EMPTY_VALUES, STAGE_END);
    expect(segments.map((s) => s.label)).toEqual(['sync', 'build', 'sync']);
  });

  it('runs an unfinished latest attempt to the end of the bounds', () => {
    const stage = stageWithProgress([{ minute: 10, message: 'building' }]);
    const segments = buildProgressSegments(stage, EMPTY_VALUES, STAGE_END);
    expect(segments[segments.length - 1].end.toMillis()).toEqual(
      STAGE_END.toMillis(),
    );
  });

  it('leaves empty space between a terminated attempt and the next attempt', () => {
    const stage = Stage.fromPartial({
      identifier: { id: 'S1' },
      attempts: [
        {
          state: StageAttemptState.STAGE_ATTEMPT_STATE_INCOMPLETE,
          stateHistory: [
            {
              state: StageAttemptState.STAGE_ATTEMPT_STATE_RUNNING,
              version: { ts: ts(0) },
            },
            {
              state: StageAttemptState.STAGE_ATTEMPT_STATE_INCOMPLETE,
              version: { ts: ts(5) },
            },
          ],
          progress: [{ message: 'first try', version: { ts: ts(1) } }],
        },
        {
          state: StageAttemptState.STAGE_ATTEMPT_STATE_COMPLETE,
          stateHistory: [
            {
              state: StageAttemptState.STAGE_ATTEMPT_STATE_RUNNING,
              version: { ts: ts(10) },
            },
            {
              state: StageAttemptState.STAGE_ATTEMPT_STATE_COMPLETE,
              version: { ts: ts(20) },
            },
          ],
          progress: [{ message: 'second try', version: { ts: ts(11) } }],
        },
      ],
    });

    const segments = buildProgressSegments(stage, EMPTY_VALUES, STAGE_END);
    expect(
      segments.map((s) => ({
        label: s.label,
        attempt: s.attemptNumber,
        start: s.start.toISO(),
        end: s.end.toISO(),
      })),
    ).toEqual([
      {
        label: 'first try',
        attempt: 1,
        start: DateTime.fromISO(ts(0)).toISO(),
        end: DateTime.fromISO(ts(5)).toISO(),
      },
      // ts(5) -> ts(10) is an empty gap between Attempt 1 and Attempt 2
      {
        label: 'second try',
        attempt: 2,
        start: DateTime.fromISO(ts(10)).toISO(),
        end: DateTime.fromISO(ts(20)).toISO(),
      },
    ]);
  });

  it('strips legacy build prefixes for worknode stages', () => {
    const buildStage = Stage.fromPartial({
      identifier: { id: 'S1', isWorknode: true },
      legacy: { worknode: { digest: 'd-build' } },
      attempts: [
        {
          stateHistory: [
            {
              state: StageAttemptState.STAGE_ATTEMPT_STATE_RUNNING,
              version: { ts: ts(0) },
            },
          ],
          progress: [
            {
              message: 'Build foo for node N1 has started building',
              version: { ts: ts(0) },
            },
          ],
        },
      ],
    });
    const buildMap = new Map([
      [
        'd-build',
        ValueData.fromPartial({
          json: {
            value: JSON.stringify({ workExecutorType: 'SUBMITTED_BUILD' }),
          },
        }),
      ],
    ]);
    expect(
      buildProgressSegments(buildStage, buildMap, STAGE_END).map(
        (s) => s.label,
      ),
    ).toEqual(['started building']);
  });

  it('extracts ATP machine-readable states from progress details and ignores COMPLETED', () => {
    const atpStage = Stage.fromPartial({
      identifier: { id: 'S2', isWorknode: true },
      attempts: [
        {
          stateHistory: [
            {
              state: StageAttemptState.STAGE_ATTEMPT_STATE_RUNNING,
              version: { ts: ts(0) },
            },
            {
              state: StageAttemptState.STAGE_ATTEMPT_STATE_COMPLETE,
              version: { ts: ts(30) },
            },
          ],
          progress: [
            {
              message: '',
              version: { ts: ts(0) },
              details: [
                {
                  typeUrl:
                    'type.googleapis.com/wireless.android.launchcontrol.WorkNode.ProgressMessage.AtpMachineReadableMessage',
                  digest: 'd-queued',
                },
              ],
            },
            {
              message: '',
              version: { ts: ts(10) },
              details: [
                {
                  typeUrl:
                    'type.googleapis.com/wireless.android.launchcontrol.WorkNode.ProgressMessage.AtpMachineReadableMessage',
                  digest: 'd-running',
                },
              ],
            },
            {
              message: '',
              version: { ts: ts(30) },
              details: [
                {
                  typeUrl:
                    'type.googleapis.com/wireless.android.launchcontrol.WorkNode.ProgressMessage.AtpMachineReadableMessage',
                  digest: 'd-completed',
                },
              ],
            },
          ],
        },
      ],
    });

    const atpMap = new Map([
      [
        'd-queued',
        ValueData.fromPartial({
          json: { value: JSON.stringify({ state: 'QUEUED' }) },
        }),
      ],
      [
        'd-running',
        ValueData.fromPartial({
          json: { value: JSON.stringify({ state: 'RUNNING' }) },
        }),
      ],
      [
        'd-completed',
        ValueData.fromPartial({
          json: { value: JSON.stringify({ state: 'COMPLETED' }) },
        }),
      ],
    ]);

    const segments = buildProgressSegments(atpStage, atpMap, STAGE_END);
    expect(
      segments.map((s) => ({
        label: s.label,
        start: s.start.toISO(),
        end: s.end.toISO(),
      })),
    ).toEqual([
      {
        label: 'queued',
        start: DateTime.fromISO(ts(0)).toISO(),
        end: DateTime.fromISO(ts(10)).toISO(),
      },
      {
        label: 'running',
        start: DateTime.fromISO(ts(10)).toISO(),
        end: DateTime.fromISO(ts(30)).toISO(),
      },
    ]);
  });
});

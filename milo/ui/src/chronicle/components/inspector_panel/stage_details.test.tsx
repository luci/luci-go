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

import { render, screen } from '@testing-library/react';

import { TYPE_URL_LEGACY_WORKNODE_STAGE } from '@/chronicle/utils/legacy_worknode';
import { OmitReason } from '@/proto/turboci/graph/orchestrator/v1/omit_reason.pb';
import { Stage } from '@/proto/turboci/graph/orchestrator/v1/stage.pb';
import { StageAttemptState } from '@/proto/turboci/graph/orchestrator/v1/stage_attempt_state.pb';
import { StageState } from '@/proto/turboci/graph/orchestrator/v1/stage_state.pb';
import { ValueData } from '@/proto/turboci/graph/orchestrator/v1/value_data.pb';

import { StageDetails } from './stage_details';
import { RenderMode } from './types';

describe('StageDetails', () => {
  it('renders stage metadata correctly for S-stages', () => {
    const stage = Stage.fromPartial({
      identifier: { id: 'build-stage-1', isWorknode: false },
      state: StageState.STAGE_STATE_FINAL,
      realm: 'android:ci',
    });
    const valueDataMap = new Map<string, ValueData>();

    render(<StageDetails view={stage} valueDataMap={valueDataMap} />);

    expect(screen.getByText('ID')).toBeInTheDocument();
    expect(screen.getByText(':Sbuild-stage-1')).toBeInTheDocument();
    expect(screen.queryByText('Label')).not.toBeInTheDocument();
    expect(screen.getByText('State')).toBeInTheDocument();
    expect(screen.getByText('STAGE_STATE_FINAL')).toBeInTheDocument();
    expect(screen.getByText('Realm')).toBeInTheDocument();
    expect(screen.getByText('android:ci')).toBeInTheDocument();
  });

  it('renders Label for N-stages', () => {
    const stage = Stage.fromPartial({
      identifier: { id: 'worknode-stage-1', isWorknode: true },
      state: StageState.STAGE_STATE_FINAL,
    });
    const valueDataMap = new Map<string, ValueData>();

    render(<StageDetails view={stage} valueDataMap={valueDataMap} />);

    expect(screen.getByText('Label')).toBeInTheDocument();
    expect(screen.getByText('Stage: worknode-stage-1')).toBeInTheDocument();
  });

  it('renders Work Output section for legacy worknode with workOutput and displays worknode label', () => {
    const legacyJson = JSON.stringify({
      workExecutorType: 'ATP_TEST',
      workParameters: {
        atpTestParameters: { testName: 'CtsOsTestCases' },
      },
      workOutput: {
        status: 'PASSED',
        passedCount: 42,
      },
    });

    const stage = Stage.fromPartial({
      identifier: { id: 'test-stage-1', isWorknode: true },
      state: StageState.STAGE_STATE_FINAL,
      args: {
        typeUrl: TYPE_URL_LEGACY_WORKNODE_STAGE,
        digest: 'wn-digest-1',
      },
      legacy: {
        worknode: {
          digest: 'wn-digest-1',
        },
      },
      attempts: [
        {
          state: StageAttemptState.STAGE_ATTEMPT_STATE_COMPLETE,
          details: [],
        },
      ],
    });

    const valueDataMap = new Map<string, ValueData>([
      [
        'wn-digest-1',
        ValueData.fromPartial({
          json: { value: legacyJson },
        }),
      ],
    ]);

    render(<StageDetails view={stage} valueDataMap={valueDataMap} />);

    expect(screen.getByText('Label')).toBeInTheDocument();
    expect(screen.getByText('test CtsOsTestCases')).toBeInTheDocument();
    expect(screen.getByText('Work Output')).toBeInTheDocument();
    // Work output fields appear in both the "Args" section and the dedicated
    // "Work Output" section.
    expect(screen.getAllByText(/status/)).toHaveLength(2);
    expect(screen.getAllByText(/PASSED/)).toHaveLength(2);
    expect(screen.getAllByText(/passedCount/)).toHaveLength(2);
  });

  it('renders Work Output section with omitReason when legacy worknode was omitted', () => {
    const stage = Stage.fromPartial({
      identifier: { id: 'test-stage-3' },
      state: StageState.STAGE_STATE_FINAL,
      legacy: {
        worknode: {
          omitReason: OmitReason.OMIT_REASON_NO_ACCESS,
        },
      },
    });

    const valueDataMap = new Map<string, ValueData>();

    render(<StageDetails view={stage} valueDataMap={valueDataMap} />);

    expect(screen.getByText('Work Output')).toBeInTheDocument();
    expect(
      screen.getByText(
        'Access Denied: You do not have permission to view this data.',
      ),
    ).toBeInTheDocument();
  });

  it('does not render Work Output section when no legacy work output or omit reason exists', () => {
    const stage = Stage.fromPartial({
      identifier: { id: 'non-legacy-stage' },
      state: StageState.STAGE_STATE_FINAL,
    });

    const valueDataMap = new Map<string, ValueData>();

    render(<StageDetails view={stage} valueDataMap={valueDataMap} />);

    expect(screen.queryByText('Work Output')).not.toBeInTheDocument();
  });

  it('renders in Raw JSON mode when renderMode is Json', () => {
    const stage = Stage.fromPartial({
      identifier: { id: 'stage-raw-json' },
      realm: 'chromium:ci',
    });

    const valueDataMap = new Map<string, ValueData>();

    render(
      <StageDetails
        view={stage}
        valueDataMap={valueDataMap}
        renderMode={RenderMode.Json}
      />,
    );

    expect(screen.getByText(/"stage-raw-json"/)).toBeInTheDocument();
  });

  it('renders progress messages with timestamps and details for stage attempts', () => {
    const stage = Stage.fromPartial({
      identifier: { id: 'worknode-stage-progress', isWorknode: true },
      state: StageState.STAGE_STATE_FINAL,
      attempts: [
        {
          state: StageAttemptState.STAGE_ATTEMPT_STATE_COMPLETE,
          version: { ts: '2026-09-02T00:07:52.625Z' },
          details: [],
          progress: [
            {
              message:
                'Build P135607851 for node L94500030178125231:N32900031086014236 has been inserted',
              version: { ts: '2026-09-01T23:44:32.771Z' },
              details: [],
            },
            {
              message:
                'Build P135607851 for node L94500030178125231:N32900031086014236 has begun syncing sources',
              version: { ts: '2026-09-01T23:45:59.483Z' },
              details: [],
            },
            {
              message: '',
              version: { ts: '2026-09-02T00:07:52.625Z' },
              details: [
                {
                  typeUrl:
                    'type.googleapis.com/wireless.android.launchcontrol.WorkNode.ProgressMessage.AttemptEnded',
                  omitReason: OmitReason.OMIT_REASON_NO_ACCESS,
                },
              ],
            },
          ],
        },
      ],
    });

    const valueDataMap = new Map<string, ValueData>();

    render(<StageDetails view={stage} valueDataMap={valueDataMap} />);

    expect(screen.getByText('Progress')).toBeInTheDocument();
    expect(
      screen.getByText(
        'Build P135607851 for node L94500030178125231:N32900031086014236 has been inserted',
      ),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        'Build P135607851 for node L94500030178125231:N32900031086014236 has begun syncing sources',
      ),
    ).toBeInTheDocument();
    expect(screen.getAllByText('Timestamp')).toHaveLength(3);
    expect(
      screen.getByText(
        'Access Denied: You do not have permission to view this data.',
      ),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        'Details: type.googleapis.com/wireless.android.launchcontrol.WorkNode.ProgressMessage.AttemptEnded',
      ),
    ).toBeInTheDocument();
  });

  it('renders progress details with valueData content when available', () => {
    const detailJson = JSON.stringify({ step: 'downloading', percent: 75 });
    const stage = Stage.fromPartial({
      identifier: { id: 'stage-progress-with-data' },
      state: StageState.STAGE_STATE_ATTEMPTING,
      attempts: [
        {
          state: StageAttemptState.STAGE_ATTEMPT_STATE_RUNNING,
          details: [],
          progress: [
            {
              message: 'Step in progress',
              version: { ts: '2026-09-01T12:00:00.000Z' },
              details: [
                {
                  typeUrl: 'type.googleapis.com/example.StepProgress',
                  digest: 'step-progress-digest',
                },
              ],
            },
          ],
        },
      ],
    });

    const valueDataMap = new Map<string, ValueData>([
      [
        'step-progress-digest',
        ValueData.fromPartial({
          json: { value: detailJson },
        }),
      ],
    ]);

    render(<StageDetails view={stage} valueDataMap={valueDataMap} />);

    expect(screen.getByText('Progress')).toBeInTheDocument();
    expect(screen.getByText('Step in progress')).toBeInTheDocument();
    expect(
      screen.getByText('Details: type.googleapis.com/example.StepProgress'),
    ).toBeInTheDocument();
    expect(screen.getByText(/"step": "downloading"/)).toBeInTheDocument();
    expect(screen.getByText(/"percent": 75/)).toBeInTheDocument();
  });

  it('does not render Progress section when attempt has no progress', () => {
    const stage = Stage.fromPartial({
      identifier: { id: 'stage-no-progress' },
      state: StageState.STAGE_STATE_FINAL,
      attempts: [
        {
          state: StageAttemptState.STAGE_ATTEMPT_STATE_COMPLETE,
          details: [],
          progress: [],
        },
      ],
    });

    const valueDataMap = new Map<string, ValueData>();

    render(<StageDetails view={stage} valueDataMap={valueDataMap} />);

    expect(screen.getByText('Attempt 1')).toBeInTheDocument();
    expect(screen.queryByText('Progress')).not.toBeInTheDocument();
  });
});

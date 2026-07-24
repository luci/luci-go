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

import { OmitReason } from '@/proto/turboci/graph/orchestrator/v1/omit_reason.pb';
import { Stage } from '@/proto/turboci/graph/orchestrator/v1/stage.pb';
import { StageAttemptState } from '@/proto/turboci/graph/orchestrator/v1/stage_attempt_state.pb';
import { StageState } from '@/proto/turboci/graph/orchestrator/v1/stage_state.pb';
import { ValueData } from '@/proto/turboci/graph/orchestrator/v1/value_data.pb';

import { StageDetails } from './stage_details';
import { RenderMode } from './types';

describe('StageDetails', () => {
  it('renders stage metadata correctly', () => {
    const stage = Stage.fromPartial({
      identifier: { id: 'build-stage-1' },
      state: StageState.STAGE_STATE_FINAL,
      realm: 'android:ci',
    });
    const valueDataMap = new Map<string, ValueData>();

    render(<StageDetails view={stage} valueDataMap={valueDataMap} />);

    expect(screen.getByText('ID')).toBeInTheDocument();
    expect(screen.getByText(':?build-stage-1')).toBeInTheDocument();
    expect(screen.getByText('State')).toBeInTheDocument();
    expect(screen.getByText('STAGE_STATE_FINAL')).toBeInTheDocument();
    expect(screen.getByText('Realm')).toBeInTheDocument();
    expect(screen.getByText('android:ci')).toBeInTheDocument();
  });

  it('renders Work Output section for legacy worknode with workOutput', () => {
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
      identifier: { id: 'test-stage-1' },
      state: StageState.STAGE_STATE_FINAL,
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

    expect(screen.getByText('Work Output')).toBeInTheDocument();
    expect(screen.getByText(/status/)).toBeInTheDocument();
    expect(screen.getByText(/PASSED/)).toBeInTheDocument();
    expect(screen.getByText(/passedCount/)).toBeInTheDocument();
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
});

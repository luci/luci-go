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

import { Stage_Attempt_Progress } from '@/proto/turboci/graph/orchestrator/v1/stage.pb';
import { ValueData } from '@/proto/turboci/graph/orchestrator/v1/value_data.pb';

import { StageAttemptProgress } from './stage_attempt_progress';

describe('StageAttemptProgress', () => {
  it('returns null when progress array is empty', () => {
    const { container } = render(
      <StageAttemptProgress progress={[]} valueDataMap={new Map()} />,
    );
    expect(container.firstChild).toBeNull();
  });

  it('renders progress messages with timestamps and details', () => {
    const progressList: Stage_Attempt_Progress[] = [
      {
        message: 'Task initiated',
        version: { ts: '2026-09-01T12:00:00.000Z' },
        details: [
          {
            typeUrl: 'type.googleapis.com/test.InitDetails',
            digest: 'digest-1',
          },
        ],
      },
      {
        message: 'Task running',
        version: { ts: '2026-09-01T12:05:00.000Z' },
        details: [],
      },
    ];

    const valueDataMap = new Map<string, ValueData>([
      [
        'digest-1',
        ValueData.fromPartial({
          json: { value: JSON.stringify({ status: 'ok' }) },
        }),
      ],
    ]);

    render(
      <StageAttemptProgress
        progress={progressList}
        valueDataMap={valueDataMap}
      />,
    );

    expect(screen.getByText('Task initiated')).toBeInTheDocument();
    expect(screen.getByText('Task running')).toBeInTheDocument();
    expect(screen.getAllByText('Timestamp')).toHaveLength(2);
    expect(
      screen.getByText('Details: type.googleapis.com/test.InitDetails'),
    ).toBeInTheDocument();
    expect(screen.getByText(/"status": "ok"/)).toBeInTheDocument();
  });
});

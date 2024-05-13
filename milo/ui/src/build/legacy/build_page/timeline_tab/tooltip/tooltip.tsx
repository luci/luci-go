// Copyright 2024 The LUCI Authors.
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

import { displayDuration } from '@/common/tools/time_utils';
import { Step } from '@/proto/go.chromium.org/luci/buildbucket/proto/step.pb';

import { TimestampRow } from './timestamp_row';

export interface StepTooltipProps {
  readonly step: Step;
  readonly buildStartTime: DateTime;
}

export function StepTooltip({ step, buildStartTime }: StepTooltipProps) {
  const startTime = step.startTime ? DateTime.fromISO(step.startTime) : null;
  const endTime = step.endTime ? DateTime.fromISO(step.endTime) : null;
  const duration = startTime && endTime ? endTime.diff(startTime) : null;

  return (
    <table>
      <tbody>
        {step.logs.length ? (
          <tr>
            <td colSpan={2}>Click to open associated log.</td>
          </tr>
        ) : (
          <></>
        )}

        <TimestampRow
          label="Start Time"
          timestamp={startTime}
          buildStartTime={buildStartTime}
        />
        <TimestampRow
          label="End Time"
          timestamp={endTime}
          buildStartTime={buildStartTime}
        />
        <tr>
          <td>Duration:</td>
          <td>{duration ? <>{displayDuration(duration)}</> : <>N/A</>}</td>
        </tr>
      </tbody>
    </table>
  );
}

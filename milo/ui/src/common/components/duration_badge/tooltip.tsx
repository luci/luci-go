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

import { DateTime, Duration } from 'luxon';

import { displayDuration, LONG_TIME_FORMAT } from '@/common/tools/time_utils';

export interface DurationTooltipProps {
  readonly durationLabel: string;
  readonly duration?: Duration | null;
  readonly fromLabel: string;
  readonly from?: DateTime | null;
  readonly toLabel: string;
  readonly to?: DateTime | null;
}

export function DurationTooltip({
  durationLabel,
  duration,
  fromLabel,
  from,
  toLabel,
  to,
}: DurationTooltipProps) {
  return (
    <table>
      <tbody>
        <tr>
          <td>{durationLabel}:</td>
          <td>{duration ? displayDuration(duration) : 'N/A'}</td>
        </tr>
        <tr>
          <td>{fromLabel}:</td>
          <td>{from ? from.toFormat(LONG_TIME_FORMAT) : 'N/A'}</td>
        </tr>
        <tr>
          <td>{toLabel}:</td>
          <td>{to ? to.toFormat(LONG_TIME_FORMAT) : 'N/A'}</td>
        </tr>
      </tbody>
    </table>
  );
}

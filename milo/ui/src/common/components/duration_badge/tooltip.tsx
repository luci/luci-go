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
  readonly duration?: Duration | null;
  readonly from?: DateTime | null;
  readonly to?: DateTime | null;
}

export function DurationTooltip({ duration, from, to }: DurationTooltipProps) {
  return (
    <table>
      <tbody>
        <tr>
          <td>Duration:</td>
          <td>{duration ? displayDuration(duration) : 'N/A'}</td>
        </tr>
        <tr>
          <td>From:</td>
          <td>{from ? from.toFormat(LONG_TIME_FORMAT) : 'N/A'}</td>
        </tr>
        <tr>
          <td>To:</td>
          <td>{to ? to.toFormat(LONG_TIME_FORMAT) : 'N/A'}</td>
        </tr>
      </tbody>
    </table>
  );
}

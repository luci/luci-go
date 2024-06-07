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

import { TimeZoneConfig } from './constants';

export interface TooltipProps {
  readonly datetime: DateTime;
  readonly format: string;
  readonly zones: readonly TimeZoneConfig[];
}

export function Tooltip({ datetime, format, zones }: TooltipProps) {
  const now = DateTime.now();
  return (
    <table>
      <tbody>
        <tr>
          <td colSpan={2}>{displayDuration(now.diff(datetime))} ago</td>
        </tr>
        {zones.map((tz, i) => (
          <tr key={i}>
            <td>{tz.label}:</td>
            <td>{datetime.setZone(tz.zone).toFormat(format)}</td>
            <td>{now.setZone(tz.zone).toFormat(format)}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

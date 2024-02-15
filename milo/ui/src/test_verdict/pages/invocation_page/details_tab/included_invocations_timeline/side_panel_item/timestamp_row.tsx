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

import {
  NUMERIC_TIME_FORMAT,
  displayDuration,
} from '@/common/tools/time_utils';

export interface TimestampRowProps {
  readonly label: string;
  readonly timestamp?: DateTime;
  readonly parentCreateTime: DateTime;
}

export function TimestampRow({
  label,
  timestamp,
  parentCreateTime,
}: TimestampRowProps) {
  const diff = timestamp?.diff(parentCreateTime);
  return (
    <tr>
      <td>{label}:</td>
      <td>
        {timestamp && diff ? (
          <>
            {timestamp.toFormat(NUMERIC_TIME_FORMAT)}
            {diff.toMillis() >= 0 ? (
              <> ({displayDuration(diff)} after parent was created)</>
            ) : (
              <> ({displayDuration(diff.negate())} before parent was created)</>
            )}
          </>
        ) : (
          'N/A'
        )}
      </td>
    </tr>
  );
}

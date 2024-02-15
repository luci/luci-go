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

import { CircularProgress } from '@mui/material';
import { DateTime } from 'luxon';

import { displayDuration } from '@/common/tools/time_utils';
import { Invocation } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/invocation.pb';

import { TimestampRow } from './timestamp_row';

export interface InvocationTooltipProps {
  readonly invId: string;
  readonly invocation?: Invocation;
  readonly parentCreateTime: DateTime;
}

export function InvocationTooltip({
  invId,
  invocation,
  parentCreateTime,
}: InvocationTooltipProps) {
  const createTime = invocation?.createTime
    ? DateTime.fromISO(invocation.createTime)
    : undefined;
  const finalizeTime = invocation?.finalizeTime
    ? DateTime.fromISO(invocation.finalizeTime)
    : undefined;
  return (
    <table>
      <tbody>
        <tr>
          <td colSpan={2}>Click to open invocation: {invId}</td>
        </tr>
        {invocation ? (
          <>
            <TimestampRow
              label="Create Time"
              timestamp={createTime}
              parentCreateTime={parentCreateTime}
            />
            <TimestampRow
              label="Finalize Time"
              timestamp={finalizeTime}
              parentCreateTime={parentCreateTime}
            />
            <tr>
              <td>Duration:</td>
              <td>
                {createTime && finalizeTime ? (
                  <>{displayDuration(finalizeTime.diff(createTime))}</>
                ) : (
                  <>N/A</>
                )}
              </td>
            </tr>
          </>
        ) : (
          <tr>
            <td colSpan={2}>
              <CircularProgress size={16} />
            </td>
          </tr>
        )}
      </tbody>
    </table>
  );
}

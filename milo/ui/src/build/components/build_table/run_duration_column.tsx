// Copyright 2023 The LUCI Authors.
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

import { TableCell } from '@mui/material';
import { DateTime } from 'luxon';

import { DurationBadge } from '@/common/components/duration_badge';

import { useBuild } from './context';

export function RunDurationHeadCell() {
  return (
    // Use a shorthand with a tooltip to make the column narrower.
    <TableCell width="1px" title="Run Duration">
      Run D.
    </TableCell>
  );
}

export function RunDurationContentCell() {
  const build = useBuild();
  const startTime = build.startTime ? DateTime.fromISO(build.startTime) : null;
  const endTime = build.endTime ? DateTime.fromISO(build.endTime) : null;
  const runDuration = startTime && endTime ? endTime.diff(startTime) : null;

  return (
    <TableCell>
      {runDuration ? (
        <DurationBadge
          sx={{ verticalAlign: 'text-top' }}
          duration={runDuration}
          from={startTime}
          to={endTime}
        />
      ) : (
        'N/A'
      )}
    </TableCell>
  );
}

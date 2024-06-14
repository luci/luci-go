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

export function PendingDurationHeadCell() {
  return (
    // Use a shorthand with a tooltip to make the column narrower.
    <TableCell width="1px" title="Pending Duration">
      Pending
    </TableCell>
  );
}

export function PendingDurationContentCell() {
  const build = useBuild();
  const createTime = DateTime.fromISO(build.createTime);
  const startTime = build.startTime ? DateTime.fromISO(build.startTime) : null;

  return (
    <TableCell>
      <DurationBadge
        durationLabel="Pending Duration"
        fromLabel="Schedule Time"
        from={createTime}
        toLabel="Start Time"
        to={startTime}
      />
    </TableCell>
  );
}

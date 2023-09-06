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

import { CompactTimestamp } from '@/build/components/build_table/compact_timestamp';

import { useBuild } from './context';

export function EndTimeHeadCell() {
  return <TableCell width="1px">End Time</TableCell>;
}

export function EndTimeContentCell() {
  const build = useBuild();
  const endTime = build.endTime ? DateTime.fromISO(build.endTime) : null;

  return (
    <TableCell>
      {endTime ? <CompactTimestamp datetime={endTime} /> : 'N/A'}
    </TableCell>
  );
}

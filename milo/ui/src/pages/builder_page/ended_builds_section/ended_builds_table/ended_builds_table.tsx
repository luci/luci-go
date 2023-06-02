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

import { Table, TableBody } from '@mui/material';
import { useState } from 'react';

import { Build } from '@/common/services/buildbucket';
import { ExpandableEntriesState } from '@/common/store/expandable_entries_state/expandable_entries_state';

import { EndedBuildsTableHead } from './ended_builds_table_head';
import { EndedBuildsTableRow } from './ended_builds_table_row';

export interface EndedBuildsTableProps {
  readonly endedBuilds: readonly Build[];
}

export function EndedBuildsTable({ endedBuilds }: EndedBuildsTableProps) {
  const [tableState] = useState(() => ExpandableEntriesState.create());

  return (
    <Table size="small">
      <EndedBuildsTableHead tableState={tableState} />
      <TableBody
        sx={{
          // Each <EndedBuildsTableRow /> is consist of two <tr />s. The first
          // <tr /> is the actual row while the second <tr /> contains the
          // expandable body as the first <tr />.
          // We only want to select the first <tr /> in every other
          // <EndedBuildsTableRow />.
          '& tr:nth-of-type(4n + 1)': {
            backgroundColor: 'var(--block-background-color)',
          },
        }}
      >
        {endedBuilds.map((b) => (
          <EndedBuildsTableRow key={b.id} tableState={tableState} build={b} />
        ))}
      </TableBody>
    </Table>
  );
}

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
import { useStore } from '@/common/store';
import { ExpandableEntriesState } from '@/common/store/expandable_entries_state';

import { EndedBuildsTableHead } from './ended_builds_table_head';
import { EndedBuildsTableRow } from './ended_builds_table_row';

export interface EndedBuildsTableProps {
  readonly endedBuilds: readonly Build[];
  readonly isLoading: boolean;
}

export function EndedBuildsTable({
  endedBuilds,
  isLoading,
}: EndedBuildsTableProps) {
  // The config is only used during initialization.
  // Don't need to declare the component as an observable.
  const config = useStore().userConfig.builderPage;

  const [tableState] = useState(() =>
    ExpandableEntriesState.create({
      defaultExpanded: config.expandEndedBuildsEntryByDefault,
    })
  );

  const hasChanges = endedBuilds.some((b) => b.input?.gerritChanges?.length);

  return (
    <Table
      size="small"
      sx={{
        borderCollapse: 'separate',
        '& td, th': {
          padding: '0px 8px',
        },
        minWidth: '1000px',
      }}
    >
      <EndedBuildsTableHead
        tableState={tableState}
        displayGerritChanges={hasChanges}
        isLoading={isLoading}
      />
      <TableBody
        sx={{
          // Only apply when the table body is not on hover because the row is
          // already highlighted with a similar color on hover
          '&:not(:hover) > tr:nth-of-type(odd)': {
            backgroundColor: 'var(--block-background-color)',
          },
        }}
      >
        {endedBuilds.map((b) => (
          <EndedBuildsTableRow
            key={b.id}
            tableState={tableState}
            build={b}
            displayGerritChanges={hasChanges}
          />
        ))}
      </TableBody>
    </Table>
  );
}

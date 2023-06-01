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

import { ChevronRight, ExpandMore } from '@mui/icons-material';
import { IconButton, TableCell, TableHead, TableRow } from '@mui/material';
import { observer } from 'mobx-react-lite';

import { ExpandableEntriesStateInstance } from '../../../../store/expandable_entries_state';

export interface EndedBuildsTableHeadProps {
  readonly tableState: ExpandableEntriesStateInstance;
}

export const EndedBuildsTableHead = observer(
  ({ tableState }: EndedBuildsTableHeadProps) => {
    return (
      <TableHead>
        <TableRow>
          <TableCell>
            <IconButton
              aria-label="toggle-all-rows"
              size="small"
              onClick={() => {
                tableState.toggleAll(!tableState.defaultExpanded);
              }}
            >
              {tableState.defaultExpanded ? <ExpandMore /> : <ChevronRight />}
            </IconButton>
          </TableCell>
          <TableCell>Status</TableCell>
          <TableCell>Build #</TableCell>
          <TableCell>Create Time</TableCell>
          <TableCell>End Time</TableCell>
          <TableCell>Run Duration</TableCell>
          <TableCell>Commit</TableCell>
          <TableCell>Changes</TableCell>
        </TableRow>
      </TableHead>
    );
  }
);

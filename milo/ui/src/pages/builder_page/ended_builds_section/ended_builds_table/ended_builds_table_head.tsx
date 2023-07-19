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
import {
  Box,
  IconButton,
  LinearProgress,
  TableCell,
  TableHead,
  TableRow,
} from '@mui/material';
import { action } from 'mobx';
import { observer } from 'mobx-react-lite';

import { useStore } from '@/common/store';
import { ExpandableEntriesStateInstance } from '@/common/store/expandable_entries_state/expandable_entries_state';

export interface EndedBuildsTableHeadProps {
  readonly tableState: ExpandableEntriesStateInstance;
  readonly displayGerritChanges: boolean;
  readonly isLoading: boolean;
}

export const EndedBuildsTableHead = observer(
  ({
    tableState,
    displayGerritChanges,
    isLoading,
  }: EndedBuildsTableHeadProps) => {
    const config = useStore().userConfig.builderPage;

    return (
      <TableHead
        sx={{
          position: 'sticky',
          top: 0,
          backgroundColor: 'white',
          zIndex: 2,
          '& th': {
            border: 'none',
          },
        }}
      >
        <TableRow
          sx={{
            '& > th': {
              whiteSpace: 'nowrap',
            },
          }}
        >
          {
            // Use a small width on all columns except the summary column to
            // prevent them from growing.
          }
          <TableCell width="1px" title="Status" sx={{ textAlign: 'center' }}>
            S
          </TableCell>
          <TableCell width="1px">Build #</TableCell>
          <TableCell width="1px">Create Time</TableCell>
          <TableCell width="1px">End Time</TableCell>
          <TableCell width="1px" title="Run Duration">
            Run D.
          </TableCell>
          <TableCell width="1px">Commit</TableCell>
          {displayGerritChanges && (
            <TableCell width="1px">
              <Box
                sx={{
                  display: 'grid',
                  gridTemplateColumns: '34px 1fr',
                }}
              >
                <IconButton
                  aria-label="toggle-all-rows"
                  size="small"
                  onClick={action(() => {
                    const shouldExpand = !tableState.defaultExpanded;
                    config.setExpandEndedBuildsEntryByDefault(shouldExpand);
                    tableState.toggleAll(shouldExpand);
                  })}
                >
                  {tableState.defaultExpanded ? (
                    <ExpandMore />
                  ) : (
                    <ChevronRight />
                  )}
                </IconButton>
                <Box sx={{ lineHeight: '34px' }}>Changes</Box>
              </Box>
            </TableCell>
          )}
          <TableCell>
            <Box
              sx={{
                display: 'grid',
                gridTemplateColumns: '34px 1fr',
              }}
            >
              <IconButton
                aria-label="toggle-all-rows"
                size="small"
                onClick={action(() => {
                  const shouldExpand = !tableState.defaultExpanded;
                  config.setExpandEndedBuildsEntryByDefault(shouldExpand);
                  tableState.toggleAll(shouldExpand);
                })}
              >
                {tableState.defaultExpanded ? <ExpandMore /> : <ChevronRight />}
              </IconButton>
              <Box sx={{ lineHeight: '34px' }}>Summary</Box>
            </Box>
          </TableCell>
        </TableRow>
        <TableRow>
          <TableCell
            colSpan={100}
            className="divider"
            sx={{ '&.divider': { padding: '0' } }}
          >
            <LinearProgress
              value={100}
              variant={isLoading ? 'indeterminate' : 'determinate'}
              color={isLoading ? 'primary' : 'dividerLine'}
              sx={{ height: '1px' }}
            />
          </TableCell>
        </TableRow>
      </TableHead>
    );
  }
);

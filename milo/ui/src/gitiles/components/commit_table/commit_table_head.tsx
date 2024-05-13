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
import { ReactNode } from 'react';
import { useHotkeys } from 'react-hotkeys-hook';

import { useDefaultExpandedState } from './context';

export interface CommitTableHeadProps {
  readonly toggleExpandHotkey?: string;
  readonly children: ReactNode;
}

export function CommitTableHead({
  toggleExpandHotkey,
  children,
}: CommitTableHeadProps) {
  const [defaultExpanded, setDefaultExpanded] = useDefaultExpandedState();
  useHotkeys(
    toggleExpandHotkey ? [toggleExpandHotkey] : [],
    () => setDefaultExpanded((expanded) => !expanded),
    [setDefaultExpanded],
  );

  return (
    <TableHead
      sx={{
        position: 'sticky',
        top: 'var(--accumulated-top)',
        backgroundColor: 'white',
        zIndex: 2,
      }}
    >
      <TableRow
        sx={{
          '& > th': {
            whiteSpace: 'nowrap',
          },
        }}
      >
        <TableCell width="1px">
          <IconButton
            aria-label="toggle-all-rows"
            size="small"
            onClick={() => setDefaultExpanded(!defaultExpanded)}
            title={
              toggleExpandHotkey
                ? `Press "${toggleExpandHotkey}" to toggle all entries.`
                : ''
            }
          >
            {defaultExpanded ? <ExpandMore /> : <ChevronRight />}
          </IconButton>
        </TableCell>
        {children}
      </TableRow>
    </TableHead>
  );
}

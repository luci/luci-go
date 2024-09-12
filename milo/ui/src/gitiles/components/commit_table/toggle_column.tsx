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

import { ChevronRight, ExpandMore } from '@mui/icons-material';
import { Box, IconButton, TableCell } from '@mui/material';
import { useHotkeys } from 'react-hotkeys-hook';

import {
  useDefaultExpanded,
  useExpanded,
  useSetDefaultExpanded,
  useSetExpanded,
} from './context';

export interface ToggleHeadCellProps {
  readonly hotkey?: string;
  readonly onToggle?: (expand: boolean) => void;
}

export function ToggleHeadCell({ hotkey, onToggle }: ToggleHeadCellProps) {
  const defaultExpanded = useDefaultExpanded();
  const setDefaultExpanded = useSetDefaultExpanded();
  useHotkeys(
    hotkey ? [hotkey] : [],
    () => setDefaultExpanded((expanded) => !expanded),
    [setDefaultExpanded],
  );

  return (
    <TableCell width="1px" colSpan={2}>
      <IconButton
        aria-label="toggle-all-rows"
        size="small"
        onClick={() => {
          setDefaultExpanded(!defaultExpanded);
          onToggle?.(!defaultExpanded);
        }}
        title={hotkey ? `Press "${hotkey}" to toggle all entries.` : ''}
      >
        {defaultExpanded ? <ExpandMore /> : <ChevronRight />}
      </IconButton>
    </TableCell>
  );
}

export interface ToggleContentCellProps {
  readonly onToggle?: (expand: boolean) => void;
}

export function ToggleContentCell({ onToggle }: ToggleContentCellProps) {
  const expanded = useExpanded();
  const setExpanded = useSetExpanded();

  return (
    <>
      <TableCell
        // Span through the content row so we can
        // 1. display a vertical ruler on the side of the content, and
        // 2. hide a section of the bottom border between the title row and the
        //    content row.
        // This makes it easier to tell that the title row and the content are
        // grouped together.
        rowSpan={2}
        sx={{ height: 'inherit' }}
        width="34px"
      >
        <Box
          sx={{
            display: 'grid',
            gridTemplateRows: 'auto 1fr',
            height: '100%',
          }}
        >
          <IconButton
            aria-label="toggle-row"
            size="small"
            onClick={() => {
              setExpanded(!expanded);
              onToggle?.(!expanded);
            }}
          >
            {expanded ? <ExpandMore /> : <ChevronRight />}
          </IconButton>
          <Box
            sx={{
              borderLeft: 'solid 1px var(--divider-color)',
              height: '100%',
              marginLeft: '16.5px',
            }}
          />
        </Box>
      </TableCell>
      {/* When the content is expanded, the expand icon button now have a larger
       ** space to render itself in. This can cause the entry row to shrink when
       ** the icon button cell is the tallest cell in the title row.
       **
       ** Add an invisible (width = 0) cell to ensure the height of the entry
       ** row does not shrink when the content is expanded.
       **/}
      <TableCell sx={{ padding: '0 !important' }} height="34px" />
    </>
  );
}

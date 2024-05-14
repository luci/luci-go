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
import { IconButton, TableCell } from '@mui/material';
import { useHotkeys } from 'react-hotkeys-hook';

import {
  useDefaultExpanded,
  useExpanded,
  useSetDefaultExpanded,
  useSetExpanded,
} from './context';

export interface ToggleHeadCellProps {
  readonly hotkey?: string;
}

export function ToggleHeadCell({ hotkey }: ToggleHeadCellProps) {
  const defaultExpanded = useDefaultExpanded();
  const setDefaultExpanded = useSetDefaultExpanded();
  useHotkeys(
    hotkey ? [hotkey] : [],
    () => setDefaultExpanded((expanded) => !expanded),
    [setDefaultExpanded],
  );

  return (
    <TableCell width="1px">
      <IconButton
        aria-label="toggle-all-rows"
        size="small"
        onClick={() => setDefaultExpanded(!defaultExpanded)}
        title={hotkey ? `Press "${hotkey}" to toggle all entries.` : ''}
      >
        {defaultExpanded ? <ExpandMore /> : <ChevronRight />}
      </IconButton>
    </TableCell>
  );
}

export function ToggleContentCell() {
  const expanded = useExpanded();
  const setExpanded = useSetExpanded();

  return (
    <TableCell>
      <IconButton
        aria-label="toggle-row"
        size="small"
        onClick={() => setExpanded(!expanded)}
      >
        {expanded ? <ExpandMore /> : <ChevronRight />}
      </IconButton>
    </TableCell>
  );
}

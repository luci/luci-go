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

import { GridColDef } from '@mui/x-data-grid';

import { Platform } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import { CellWithTooltip } from '../table/cell_with_tooltip';

import { COLUMN_OVERRIDES } from './dimensions';

// This may be used later to add a 'common columns' section.
// Currently, the columns from this string will appear
// first and in the same order in the device table.
const COMMON_COLUMNS: string[] = ['id', 'dut_id', 'state'];

/**
 * The idea is to first have the visible columns, then the rest.
 * Inside each group keep COMMON_COLUMNS in the order as they appear
 * in the array.
 */
const sortingComparator = (
  a: string,
  b: string,
  visibleColumnIds: string[],
  temporaryColumnIds: string[],
) => {
  const aIsVisible = visibleColumnIds.includes(a) ? 1 : 0;
  const bIsVisible = visibleColumnIds.includes(b) ? 1 : 0;
  if (aIsVisible !== bIsVisible) {
    return bIsVisible - aIsVisible;
  }

  // Both visible or neither visible, check temporary status
  const aIsTemporary = temporaryColumnIds.includes(a) ? 1 : 0;
  const bIsTemporary = temporaryColumnIds.includes(b) ? 1 : 0;
  if (aIsTemporary !== bIsTemporary) {
    return bIsTemporary - aIsTemporary;
  }

  const aCommonIndex = COMMON_COLUMNS.findIndex((c) => c === a);
  const bCommonIndex = COMMON_COLUMNS.findIndex((c) => c === b);

  // Fall back to common columns
  if (aCommonIndex !== -1 && bCommonIndex !== -1) {
    return aCommonIndex < bCommonIndex ? -1 : 1; // Sort as in COMMON_COLUMNS
  }
  if (aCommonIndex !== -1 && bCommonIndex === -1) {
    return -1;
  }
  if (aCommonIndex === -1 && bCommonIndex !== -1) {
    return 1;
  }

  return a.localeCompare(b); // Sort alphabetically
};

export const getColumns = (
  columnIds: string[],
  platform: Platform,
): GridColDef[] => {
  return columnIds.map((id) => ({
    field: id,
    headerName: COLUMN_OVERRIDES[platform][id]?.displayName || id,
    editable: false,
    minWidth: 70,
    maxWidth: 700,
    sortable: COLUMN_OVERRIDES[platform][id]?.sortable ?? true,
    flex: id === 'id' ? 3 : 1,
    renderCell: (props) =>
      COLUMN_OVERRIDES[platform][id]?.renderCell?.(props) || (
        <CellWithTooltip {...props}></CellWithTooltip>
      ),
  }));
};

export const orderColumns = (
  platform: Platform,
  columnDefs: GridColDef[],
  visibleColumnIds: string[],
  temporaryColumnIds: string[] = [],
): GridColDef[] => {
  if (columnDefs.length === 0) {
    // If the columns are still not loaded show the visible ones.
    return getColumns(visibleColumnIds, platform).sort((a, b) =>
      sortingComparator(a.field, b.field, [], temporaryColumnIds),
    );
  }

  return columnDefs.sort((a, b) =>
    sortingComparator(a.field, b.field, visibleColumnIds, temporaryColumnIds),
  );
};

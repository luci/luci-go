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

import { DeviceDataCell } from './device_data_cell';
import { COLUMN_OVERRIDES } from './dimensions';

// This may be used later to add a 'common columns' section.
// Currently, the columns from this string will appear
// first and in the same order in the device table.
const COMMON_COLUMNS: string[] = ['id', 'dut_id', 'state'];

/**
 * The idea is to first have the visible columns, then the rest.
 * Inside of each group keep COMMON_COLUMNS in the order as they appear
 * in the array.
 */
const sortingComparator = (
  a: string,
  b: string,
  visibleColumnIds: string[],
) => {
  const aIsVisible = visibleColumnIds.includes(a);
  const bIsVisible = visibleColumnIds.includes(b);

  const aCommonIndex = COMMON_COLUMNS.findIndex((c) => c === a);
  const bCommonIndex = COMMON_COLUMNS.findIndex((c) => c === b);

  if (aIsVisible && !bIsVisible) {
    return -1; // a comes before b (a is visible)
  }
  if (!aIsVisible && bIsVisible) {
    return 1; // b comes before a (b is visible)
  }

  // Both visible or neither visible, then check common columns
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

export const getColumns = (columnIds: string[]): GridColDef[] => {
  return columnIds.map((id) => ({
    field: id,
    headerName: COLUMN_OVERRIDES[id]?.displayName || id,
    editable: false,
    minWidth: 70,
    maxWidth: 700,
    flex: id === 'id' ? 3 : 1,
    renderCell: (props) =>
      COLUMN_OVERRIDES[id]?.renderCell?.(props) || (
        <DeviceDataCell {...props}></DeviceDataCell>
      ),
  }));
};

export const orderColumns = (
  columnDefs: GridColDef[],
  visibleColumnIds: string[],
): GridColDef[] => {
  if (columnDefs.length === 0) {
    // If the columns are still not loaded show the visible ones.
    return getColumns(visibleColumnIds).sort((a, b) =>
      sortingComparator(a.field, b.field, []),
    );
  }

  return columnDefs.sort((a, b) =>
    sortingComparator(a.field, b.field, visibleColumnIds),
  );
};

// Copyright 2025 The LUCI Authors.
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

import { GridColDef, GridColumnVisibilityModel } from '@mui/x-data-grid';
import { useMemo } from 'react';

import { COLUMNS_PARAM_KEY } from '@/fleet/constants/param_keys';
import { colors } from '@/fleet/theme/colors';
import { Platform } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import { orderColumns } from '../device_table/columns';

import { useParamsAndLocalStorage } from './use_params_and_local_storage';

const highlightedColumnClassName = 'column-highlight';
const temporaryColumnClassName = `${highlightedColumnClassName} temporary-column-highlight`;

/**
 * Styling object for columns that are highlighted and temporarily visible due to an active filter.
 * This is applied to the DataGrid via the `sx` prop.
 */
const columnHighlightSx = {
  [`& .MuiDataGrid-columnHeader.${highlightedColumnClassName}, & .MuiDataGrid-cell.${highlightedColumnClassName}`]:
    {
      color: colors.blue[600],
      backgroundColor: colors.blue[50],
    },
  [`& .MuiDataGrid-columnHeader.${highlightedColumnClassName}:hover, & .MuiDataGrid-cell.${highlightedColumnClassName}:hover`]:
    {
      backgroundColor: colors.blue[100],
    },
  [`& .MuiDataGrid-columnHeader.temporary-column-highlight`]: {
    '& .MuiDataGrid-columnHeaderTitle::after': {
      // Add an asterisk to temporary columns.
      content: '"*"',
      marginLeft: '6px',
    },
  },
};

export type ColumnManagementConfig = {
  allColumns: readonly GridColDef[];
  highlightedColumnIds: readonly string[];
  defaultColumns: readonly string[];
  localStorageKey: string;
  preserveOrder?: boolean;
  platform?: Platform;
};

/**
 * A hook to manage the state of DataGrid columns, including visibility,
 * persistence, and temporary display based on active filters.
 *
 * It syncs the user's preferred visible columns with URL parameters and local storage.
 * If a filter is applied to a column that is not currently visible, this hook
 * will make that column temporarily visible and apply a distinct style to it.
 */
export function useColumnManagement({
  allColumns,
  highlightedColumnIds,
  defaultColumns,
  localStorageKey,
  preserveOrder = false,
  platform,
}: ColumnManagementConfig) {
  // Manages columns the user has explicitly chosen to see.
  // This state is persisted in local storage and the URL.
  const [userVisibleColumns, setUserVisibleColumns] = useParamsAndLocalStorage(
    COLUMNS_PARAM_KEY,
    localStorageKey,
    [...defaultColumns],
  );

  // Columns not visible initially but filtered on.
  const temporaryColumnIds = useMemo(
    () =>
      highlightedColumnIds.filter((col) => !userVisibleColumns.includes(col)),
    [highlightedColumnIds, userVisibleColumns],
  );

  // The visibility model required by the MUI DataGrid.
  const columnVisibilityModel = useMemo(() => {
    const model: GridColumnVisibilityModel = {};
    allColumns.forEach(
      (col) =>
        (model[col.field] =
          temporaryColumnIds.includes(col.field) ||
          userVisibleColumns.includes(col.field)),
    );
    return model;
  }, [allColumns, userVisibleColumns, temporaryColumnIds]);

  // Callback for when the user changes column visibility in the UI.
  const onColumnVisibilityModelChange = (
    newModel: GridColumnVisibilityModel,
  ) => {
    const newVisible = Object.keys(newModel).filter((field) =>
      // Prevent temporary columns from being saved to the user's preferences.
      temporaryColumnIds.includes(field) ? false : newModel[field],
    );
    setUserVisibleColumns(newVisible);
  };

  // Generates the final column definitions for the DataGrid.
  // It adds special properties (e.g., class names) to temporary columns.
  const columns = useMemo(() => {
    const styledColumns = allColumns.map((colDef) => {
      if (temporaryColumnIds.includes(colDef.field)) {
        return {
          ...colDef,
          hideable: false,
          headerClassName: temporaryColumnClassName,
          cellClassName: temporaryColumnClassName,
        };
      }
      if (highlightedColumnIds.includes(colDef.field)) {
        return {
          ...colDef,
          headerClassName: highlightedColumnClassName,
          cellClassName: highlightedColumnClassName,
        };
      }
      return colDef;
    });
    return preserveOrder || !platform
      ? styledColumns
      : orderColumns(styledColumns, userVisibleColumns, temporaryColumnIds);
  }, [
    allColumns,
    highlightedColumnIds,
    userVisibleColumns,
    temporaryColumnIds,
    preserveOrder,
    platform,
  ]);

  // Callback to explicitly add a temporary column to user defaults.
  const addUserVisibleColumn = (column: string) => {
    if (!userVisibleColumns.includes(column)) {
      setUserVisibleColumns([...userVisibleColumns, column]);
    }
  };
  return {
    columns,
    columnVisibilityModel,
    onColumnVisibilityModelChange,
    addUserVisibleColumn,
    resetDefaultColumns: () => setUserVisibleColumns([...defaultColumns]),
    temporaryColumns: temporaryColumnIds,
    temporaryColumnSx: columnHighlightSx,
  };
}
